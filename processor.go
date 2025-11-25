package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcessingConfig holds configuration for file processing
type ProcessingConfig struct {
	SrcFolder  string
	DestFolder string
}

// CarrierPayload Carrier Payload
type FileInfo struct {
	name          string
	size          int64
	modifiedAt    time.Time
	modifiedAtStr string
	srcPath       string
	destPath      string
	hash          string
	copied        bool
	thumbnailPath string
	metadata      string
	fileType      string
	tags          []string
}

type ScanStatus struct {
	Status      string    `json:"status"`      // idle, scanning, processing, completed, error
	TotalFiles  int64     `json:"totalFiles"`  // Total files found
	Processed   int64     `json:"processed"`   // Files processed
	Copied      int64     `json:"copied"`      // Files successfully copied
	Skipped     int64     `json:"skipped"`     // Files skipped (already copied)
	Failed      int64     `json:"failed"`      // Files that failed
	StartTime   time.Time `json:"startTime"`   // When sync started
	EndTime     time.Time `json:"endTime"`     // When sync ended (if completed)
	CurrentFile string    `json:"currentFile"` // Current file being processed
	Error       string    `json:"error"`       // Error message if status is error
}

var scanStatus *ScanStatus = &ScanStatus{
	Status:     "idle",
	TotalFiles: 0,
	Processed:  0,
	Copied:     0,
	Skipped:    0,
	Failed:     0,
}

func GetScanStatus() *ScanStatus {
	return scanStatus
}

func updateScanStatus(fileInfo FileInfo, scanStatus *ScanStatus) {
	scanStatus.Status = "processing"
	scanStatus.TotalFiles++
	scanStatus.Processed++
	scanStatus.CurrentFile = fileInfo.name
	if fileInfo.copied {
		scanStatus.Skipped++
	} else {
		scanStatus.Copied++
	}
}

// String implements fmt.Stringer interface for FileInfo
func (fi FileInfo) String() string {
	copiedStr := "false"
	if fi.copied {
		copiedStr = "true"
	}
	tagsStr := strings.Join(fi.tags, ",")
	return fmt.Sprintf(`
----------------------------------------
FileInfo:
  name:          %q
  size:          %d
  modifiedAt:    %s
  srcPath:       %q
  destPath:      %q
  hash:          %s
  copied:        %s
  fileType:      %q
  thumbnailPath: %q
  tags:          %q
----------------------------------------`,
		fi.name, fi.size, fi.modifiedAtStr, fi.srcPath, fi.destPath, fi.hash, copiedStr, fi.fileType, fi.thumbnailPath, tagsStr)
}

func walkFiles(db *DB, config ProcessingConfig, incomingIDsToDelete *[]int64, fileInfoChan chan<- FileInfo) error {
	fmt.Println("Walking files from", config.SrcFolder)
	return filepath.Walk(config.SrcFolder,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			modTime := info.ModTime()
			dstPath := filepath.Join(config.DestFolder, strconv.Itoa(modTime.Year()), modTime.Month().String(), info.Name())

			hash, err := computeFileHash(path)
			if err != nil {
				return fmt.Errorf("failed to compute hash for %s: %w", path, err)
			}
			fmt.Println("computed hash for", path, hash)
			// Check if already in outcoming by hash to determine copied status
			_, _, exists, err := db.findOutcomingByHash(hash)
			if err != nil {
				return fmt.Errorf("failed to find outcoming by hash for %s: %w", path, err)
			}

			// Determine file type
			fileType := getFileType(path)

			fileInfo := FileInfo{
				name:          info.Name(),
				size:          info.Size(),
				modifiedAt:    modTime,
				modifiedAtStr: modTime.Format("2006-January-02"),
				srcPath:       path,
				destPath:      dstPath,
				hash:          hash,
				copied:        exists,
				fileType:      fileType,
				tags:          []string{},
			}

			incomingID, ierr := db.insertIncomingRecord(fileInfo)
			if ierr != nil {
				return fmt.Errorf("failed to insert incoming record for %s: %w", path, ierr)
			}
			// If already copied, skip file processing
			if fileInfo.copied {
				fileInfoChan <- fileInfo
				return nil
			}

			// Generate thumbnail and get thumbnail path
			thumbnailPath, terr := processThumbnail(dstPath, config.DestFolder)
			if terr != nil {
				// Don't fail the copy operation if thumbnail generation fails
				fmt.Println("thumbnail generation failed for", dstPath, ":", terr)
			}

			// Update FileInfo with thumbnail path
			fileInfo.thumbnailPath = thumbnailPath

			// Build metadata (EXIF for images, XMP/EXIF for videos) and store JSON in DB
			metadataJSON := BuildMetadataJSON(dstPath)
			fileInfo.metadata = metadataJSON

			err = ensureDirectory(filepath.Dir(dstPath))
			if err != nil {
				if incomingID > 0 {
					_ = db.markIncomingFailure(incomingID, fmt.Sprintf("mkdir failed: %v", err))
				}
				return fmt.Errorf("failed to create destination directory for %s: %w", path, err)
			}

			if err := copyFile(path, dstPath, info.Name(), db, incomingID); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", path, err)
			}

			*incomingIDsToDelete = append(*incomingIDsToDelete, incomingID)

			if _, err := db.insertOutcomingRecord(fileInfo); err != nil {
				return fmt.Errorf("failed to insert outcoming record for %s: %w", path, err)
			}

			// Send fileInfo to channel
			fileInfoChan <- fileInfo

			return nil
		})
}

func fileProcessing(config ProcessingConfig) error {
	// Initialize destination directory
	if err := ensureDirectory(config.DestFolder); err != nil {
		return fmt.Errorf("failed to initialize destination directory: %w", err)
	}

	// Initialize database
	db, err := initializeDB(config.DestFolder)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	incomingIDsToDelete := make([]int64, 0, 128)

	// Channel to receive error from goroutine
	errChan := make(chan error, 1)
	// Channel to receive FileInfo from goroutine
	fileInfoChan := make(chan FileInfo, 128)

	fileInfoArr := make([]FileInfo, 0, 128)

	// Run walkFiles in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(fileInfoChan)
		if err := walkFiles(db, config, &incomingIDsToDelete, fileInfoChan); err != nil {
			errChan <- fmt.Errorf("failed to walk source directory: %w", err)
			return
		}
		errChan <- nil
	}()

	// Receive FileInfo as they come in and update status
	var printWg sync.WaitGroup
	printWg.Add(1)

	go func() {
		defer printWg.Done()
		for fileInfo := range fileInfoChan {
			fileInfoArr = append(fileInfoArr, fileInfo)
			updateScanStatus(fileInfo, scanStatus)
		}
	}()

	go func() {
		// Wait for walk goroutine to complete
		wg.Wait()

		// Wait for printing goroutine to complete (channel is closed, so it will finish processing)
		printWg.Wait()

		// Check for errors (goroutine has completed, so channel will have a value)
		if walkErr := <-errChan; walkErr != nil {
			fmt.Println("failed to walk source directory:", walkErr)
		}

		// Delete all incoming records in a single batch operation
		if len(incomingIDsToDelete) > 0 {
			if err := db.deleteIncomingByIDs(incomingIDsToDelete); err != nil {
				fmt.Println("failed to delete incoming records:", err)
			}
		}
		defer db.Close()
	}()

	return nil
}

// ensureDirectory creates a directory if it doesn't exist
// Returns an error if directory creation fails
func ensureDirectory(dirPath string) error {
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		fmt.Println("mkdir failed:", dirPath, err)
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}
	return nil
}

// copyFile copies a file from source to destination
// Returns an error if the copy fails
// If db and incomingID are provided, marks the incoming record as failed on error
func copyFile(srcPath, dstPath, fileName string, db *DB, incomingID int64) error {
	existingFile, err := os.Open(srcPath)
	if err != nil {
		fmt.Println("open src failed:", srcPath, err)
		if db != nil && incomingID > 0 {
			_ = db.markIncomingFailure(incomingID, fmt.Sprintf("open src failed: %v", err))
		}
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer existingFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		fmt.Println("create dst failed:", dstPath, err)
		if db != nil && incomingID > 0 {
			_ = db.markIncomingFailure(incomingID, fmt.Sprintf("create dst failed: %v", err))
		}
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, existingFile); err != nil {
		fmt.Println("Unable to copy file --> ", fileName, err.Error())
		if db != nil && incomingID > 0 {
			_ = db.markIncomingFailure(incomingID, fmt.Sprintf("copy failed: %v", err))
		}
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// initializeDB opens and initializes the SQLite database connection
// Returns the database connection and any error encountered
func initializeDB(destFolder string) (*DB, error) {
	// DB path is always in destFolder
	dbPath := filepath.Join(destFolder, "photoManager.db")

	// Open and initialize SQLite database
	db, err := openAndInitDB(dbPath)
	if err != nil {
		fmt.Println("Failed to open/init DB:", err)
		return nil, fmt.Errorf("failed to open/init database: %w", err)
	}
	return db, nil
}

func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// getFileType determines if a file is an image, video, text, or other based on its extension
func getFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if isImageExt(ext) {
		return "image"
	} else if isVideoExt(ext) {
		return "video"
	}
	return "other"
}
