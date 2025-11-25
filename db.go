package main

import (
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps sql.DB to add custom methods
type DB struct {
	*sql.DB
}

func openAndInitDB(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	// Set connection pool settings
	sqlDB.SetMaxOpenConns(1) // SQLite works best with single connection
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0) // Keep connections open

	db := &DB{sqlDB}

	// Create new schema: incoming and outcoming tables
	schema := `
CREATE TABLE IF NOT EXISTS incoming (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	size INTEGER NOT NULL,
	modified_at TEXT NOT NULL,
	src_path TEXT NOT NULL,
	hash TEXT NOT NULL,
	copied INTEGER NOT NULL DEFAULT 0,
	file_type TEXT NOT NULL DEFAULT 'other',
	error TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS outcoming (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	size INTEGER NOT NULL,
	modified_at TEXT NOT NULL,
	src_path TEXT NOT NULL,
	dest_path TEXT NOT NULL,
	copied_at TEXT NOT NULL,
	hash TEXT NOT NULL,
	file_type TEXT NOT NULL DEFAULT 'other',
	metadata JSON NOT NULL DEFAULT '{}',
	thumbnail_path TEXT DEFAULT '',
	tags TEXT
);`
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, err
	}

	// Migrate status column to copied column if status exists
	var statusCol int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('incoming') WHERE name='status'`).Scan(&statusCol)
	if statusCol > 0 {
		// Migrate status values to copied: 'copied' -> 1, others -> 0
		_, _ = sqlDB.Exec(`UPDATE incoming SET copied = CASE WHEN status = 'copied' THEN 1 ELSE 0 END`)
		// Drop status column (SQLite doesn't support DROP COLUMN directly, so we'll recreate the table)
		// For now, we'll leave status column and just use copied. In a future migration we can drop it.
	}
	// Migrate from legacy exif_json column if present
	var exifCol int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('outcoming') WHERE name='exif_json'`).Scan(&exifCol)
	if exifCol > 0 {
		_, _ = sqlDB.Exec(`UPDATE outcoming SET metadata = exif_json WHERE (metadata IS NULL OR metadata = '' OR metadata = '{}') AND exif_json IS NOT NULL AND exif_json <> ''`)
	}
	// Ensure thumbnail_path column exists in outcoming table
	var thumbCol int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('outcoming') WHERE name='thumbnail_path'`).Scan(&thumbCol)
	if thumbCol == 0 {
		_, _ = sqlDB.Exec(`ALTER TABLE outcoming ADD COLUMN thumbnail_path TEXT DEFAULT ''`)
	}
	// Ensure tags column exists in outcoming table
	var tagsCol int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('outcoming') WHERE name='tags'`).Scan(&tagsCol)
	if tagsCol == 0 {
		_, _ = sqlDB.Exec(`ALTER TABLE outcoming ADD COLUMN tags TEXT`)
	}
	// Best-effort unique indexes on hash (ignore errors if duplicates exist)
	_, _ = sqlDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_incoming_hash ON incoming(hash)`)
	_, _ = sqlDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_outcoming_hash ON outcoming(hash)`)

	// Migrate from legacy "files" table to "outcoming" if present
	var legacyCount int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='files'`).Scan(&legacyCount); err == nil && legacyCount > 0 {
		// Ensure outcoming exists (already created above), then copy rows
		_, _ = sqlDB.Exec(`
INSERT INTO outcoming (name, size, modified_at, src_path, dest_path, copied_at, hash)
SELECT name, size, modified_at, src_path, dest_path, copied_at, '' FROM files;
`)
		// Drop legacy table after migration
		_, _ = sqlDB.Exec(`DROP TABLE files;`)
	}
	return db, nil
}

func (db *DB) clearDBTables() error {
	if _, err := db.Exec(`DELETE FROM incoming`); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM outcoming`); err != nil {
		return err
	}
	return nil
}

func (db *DB) insertIncomingRecord(fi FileInfo) (int64, error) {
	now := time.Now().Format(time.RFC3339)
	// Convert copied bool to int (0 or 1)
	copiedInt := 0
	if fi.copied {
		copiedInt = 1
	}
	// Upsert by hash using ON CONFLICT to guarantee single row per hash
	_, err := db.Exec(
		`INSERT INTO incoming (hash, name, size, modified_at, src_path, copied, file_type, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(hash) DO UPDATE SET
  name=excluded.name,
  size=excluded.size,
  modified_at=excluded.modified_at,
  src_path=excluded.src_path,
  copied=excluded.copied,
  file_type=excluded.file_type,
  error=NULL,
  updated_at=excluded.updated_at`,
		fi.hash,
		fi.name,
		fi.size,
		fi.modifiedAt.Format(time.RFC3339),
		fi.srcPath,
		copiedInt,
		fi.fileType,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	// Return the id of the (now) current row for this hash
	var id int64
	if err := db.QueryRow(`SELECT id FROM incoming WHERE hash = ?`, fi.hash).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (db *DB) markIncomingFailure(id int64, reason string) error {
	// Retry logic for SQLITE_BUSY errors
	maxRetries := 3
	var err error
	for i := 0; i < maxRetries; i++ {
		_, err = db.Exec(`UPDATE incoming SET copied=0, error=?, updated_at=? WHERE id=?`, reason, time.Now().Format(time.RFC3339), id)
		if err == nil {
			return nil
		}
		// Check if it's a busy error
		if errStr := err.Error(); !strings.Contains(errStr, "database is locked") && !strings.Contains(errStr, "SQLITE_BUSY") {
			return err
		}
		// Wait before retry (exponential backoff)
		time.Sleep(time.Duration(i+1) * 50 * time.Millisecond)
	}
	return err
}

func (db *DB) deleteIncomingByIDs(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	// Retry logic for SQLITE_BUSY errors
	maxRetries := 3
	var err error
	for i := 0; i < maxRetries; i++ {
		// Build query with placeholders
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for j, id := range ids {
			placeholders[j] = "?"
			args[j] = id
		}
		query := `DELETE FROM incoming WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		_, err = db.Exec(query, args...)
		if err == nil {
			return nil
		}
		// Check if it's a busy error
		if errStr := err.Error(); !strings.Contains(errStr, "database is locked") && !strings.Contains(errStr, "SQLITE_BUSY") {
			return err
		}
		// Wait before retry (exponential backoff)
		time.Sleep(time.Duration(i+1) * 50 * time.Millisecond)
	}
	return err
}

func (db *DB) insertOutcomingRecord(fi FileInfo) (int64, error) {
	// Convert tags array to comma-separated string
	tagsStr := strings.Join(fi.tags, ",")
	if tagsStr == "" {
		tagsStr = ""
	}

	stmt := `INSERT INTO outcoming (name, size, modified_at, src_path, dest_path, copied_at, hash, file_type, metadata, thumbnail_path, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(stmt,
		fi.name,
		fi.size,
		fi.modifiedAt.Format(time.RFC3339),
		fi.srcPath,
		fi.destPath,
		time.Now().Format(time.RFC3339),
		fi.hash,
		fi.fileType,
		fi.metadata,
		fi.thumbnailPath,
		tagsStr,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) findOutcomingByHash(hash string) (int64, string, bool, error) {
	var id int64
	var destPath string
	err := db.QueryRow(`SELECT id, dest_path FROM outcoming WHERE hash = ? LIMIT 1`, hash).Scan(&id, &destPath)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return id, destPath, true, nil
}

// Listing helpers for API
type IncomingRow struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
	SrcPath    string `json:"srcPath"`
	Hash       string `json:"hash"`
	Copied     bool   `json:"copied"`
	FileType   string `json:"fileType"`
	Error      string `json:"error"`
	UpdatedAt  string `json:"updatedAt"`
}

type OutcomingRow struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Size          int64    `json:"size"`
	ModifiedAt    string   `json:"modifiedAt"`
	SrcPath       string   `json:"srcPath"`
	DestPath      string   `json:"destPath"`
	CopiedAt      string   `json:"copiedAt"`
	FileType      string   `json:"fileType"`
	Metadata      string   `json:"metadata"`
	ThumbnailPath string   `json:"thumbnailPath,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

func (db *DB) listIncomingRows(offset, limit int64) ([]IncomingRow, error) {
	rows, err := db.Query(`SELECT id, name, size, modified_at, src_path, hash, copied, file_type, IFNULL(error,''), updated_at FROM incoming ORDER BY id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IncomingRow
	for rows.Next() {
		var r IncomingRow
		var copiedInt int
		if err := rows.Scan(&r.ID, &r.Name, &r.Size, &r.ModifiedAt, &r.SrcPath, &r.Hash, &copiedInt, &r.FileType, &r.Error, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Copied = copiedInt == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) listOutcomingRows(offset, limit int64) ([]OutcomingRow, error) {
	rows, err := db.Query(`SELECT id, name, size, modified_at, src_path, dest_path, copied_at, file_type, metadata, IFNULL(thumbnail_path,''), IFNULL(tags,'') FROM outcoming ORDER BY id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OutcomingRow
	for rows.Next() {
		var r OutcomingRow
		var thumbnailPath string
		var tagsStr string
		if err := rows.Scan(&r.ID, &r.Name, &r.Size, &r.ModifiedAt, &r.SrcPath, &r.DestPath, &r.CopiedAt, &r.FileType, &r.Metadata, &thumbnailPath, &tagsStr); err != nil {
			return nil, err
		}
		// Use stored thumbnail path from database
		if thumbnailPath != "" {
			r.ThumbnailPath = thumbnailPath
		}
		// Parse tags from comma-separated string
		if tagsStr != "" {
			r.Tags = strings.Split(tagsStr, ",")
		} else {
			r.Tags = []string{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) getOutcomingByIDRow(id int64) (*OutcomingRow, error) {
	var r OutcomingRow
	var thumbnailPath string
	var tagsStr string
	err := db.QueryRow(`SELECT id, name, size, modified_at, src_path, dest_path, copied_at, file_type, metadata, IFNULL(thumbnail_path,''), IFNULL(tags,'') FROM outcoming WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Size, &r.ModifiedAt, &r.SrcPath, &r.DestPath, &r.CopiedAt, &r.FileType, &r.Metadata, &thumbnailPath, &tagsStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Use stored thumbnail path from database
	if thumbnailPath != "" {
		r.ThumbnailPath = thumbnailPath
	}
	// Parse tags from comma-separated string
	if tagsStr != "" {
		r.Tags = strings.Split(tagsStr, ",")
	} else {
		r.Tags = []string{}
	}
	return &r, nil
}

func (db *DB) updateTags(id int64, tags []string) error {
	// Convert tags array to comma-separated string
	tagsStr := strings.Join(tags, ",")

	// Update tags
	result, err := db.Exec(`UPDATE outcoming SET tags = ? WHERE id = ?`, tagsStr, id)
	if err != nil {
		return err
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
