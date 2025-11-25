# photoManager

A simple CLI to organize files (e.g., photos) by last-modified year/month, with a SQLite ledger that tracks incoming and successfully copied files. Files are grouped into `<dest>/<YYYY>/<Month>/filename`, deduplicated by SHA‑256 hash, and every action is logged in the database.

## Requirements

- Go 1.17+
- No CGO required (uses `modernc.org/sqlite`)

If you prefer the CGO-backed driver, you can switch to `github.com/mattn/go-sqlite3`.

## Build

```bash
cd photoManager
go mod tidy
go build
```

## Usage

```bash
go run ./photoManager \
  -src "/path/to/incoming" \
  -dest "/path/to/outcoming" \
  -db "/path/to/photoManager.db" \
  -print
```

### Flags

- `-src` string: Source directory to scan (default: `~/personal/photos/incoming`)
- `-dest` string: Destination base directory (default: `~/personal/photos/outcoming`)
- `-db` string: SQLite DB file path (default: `<dest>/photoManager.db`)
- `-print`: Print processed files and dump all `incoming` and `outcoming` rows at the end
- `-clear-db`: Delete all rows from `incoming` and `outcoming` and exit

### Examples

- Copy files with DB writes:

```bash
go run ./photoManager \
  -src "/path/to/incoming" \
  -dest "/path/to/outcoming" \
  -db "/tmp/pm.sqlite" \
  -print
```

- Clear the DB and exit:

```bash
go run ./photoManager -dest "/path/to/outcoming" -db "/tmp/pm.sqlite" -clear-db
```

## What it does

1. Walks `-src` recursively and, for each file:
   - Computes SHA‑256 hash of the source file.
   - Inserts/updates an `incoming` row keyed by `hash` (upsert).
   - If that `hash` already exists in `outcoming`, marks the `incoming` row as `copied` and skips the copy.
   - Otherwise copies the file to `<dest>/<YYYY>/<Month>/filename`.
   - On copy success: deletes the `incoming` row and inserts an `outcoming` row with the same `hash`.
   - On failure: updates the `incoming` row with `copied=0` and stores the error reason.
2. With `-print`, prints all `incoming` and `outcoming` rows at the end.

## Database

- Default path: `<dest>/photoManager.db` unless overridden by `-db`.
- Tables:
  - `incoming(id, name, size, modified_at, src_path, hash, copied, error, created_at, updated_at)`
  - `outcoming(id, name, size, modified_at, src_path, dest_path, copied_at, hash)`
- Hash is used to deduplicate; a unique index on `hash` is created for both tables.
- Legacy `files` table (from earlier versions) is migrated into `outcoming` automatically if present.

## Notes

- If `-dest` is not writable you will see an error like “read-only file system.” Choose a writable destination or run with appropriate permissions.
- On macOS/Linux, ensure CGO is enabled for SQLite (e.g., `export CGO_ENABLED=1`). On Windows, a proper C toolchain is required for `mattn/go-sqlite3`. Alternatively consider switching to `modernc.org/sqlite`.