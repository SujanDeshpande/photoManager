package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

// isImageFile checks if the file extension indicates an image file
func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".tif", ".webp", ".heic", ".heif":
		return true
	}
	return false
}

// generateThumbnail generates a thumbnail for an image file
func generateThumbnail(srcPath, destPath string, maxSize int) error {
	// Open the source image
	srcImg, err := imaging.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}

	// Get image bounds
	bounds := srcImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate thumbnail dimensions maintaining aspect ratio
	var thumbWidth, thumbHeight int
	if width > height {
		// Landscape: width is the limiting factor
		thumbWidth = maxSize
		thumbHeight = int(float64(height) * float64(maxSize) / float64(width))
	} else {
		// Portrait or square: height is the limiting factor
		thumbHeight = maxSize
		thumbWidth = int(float64(width) * float64(maxSize) / float64(height))
	}

	// Resize the image
	thumbImg := imaging.Resize(srcImg, thumbWidth, thumbHeight, imaging.Lanczos)

	// Ensure the destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create thumbnail directory: %w", err)
	}

	// Save the thumbnail
	// Use JPEG format for thumbnails (smaller file size)
	// If original is PNG with transparency, we might want to preserve it, but JPEG is more common for thumbnails
	ext := strings.ToLower(filepath.Ext(destPath))
	if ext == ".png" {
		err = imaging.Save(thumbImg, destPath)
	} else {
		// Default to JPEG for thumbnails
		if !strings.HasSuffix(strings.ToLower(destPath), ".jpg") && !strings.HasSuffix(strings.ToLower(destPath), ".jpeg") {
			destPath = destPath[:len(destPath)-len(ext)] + ".jpg"
		}
		err = imaging.Save(thumbImg, destPath, imaging.JPEGQuality(85))
	}

	if err != nil {
		return fmt.Errorf("failed to save thumbnail: %w", err)
	}

	return nil
}

// processThumbnail generates a thumbnail for an image file and returns the relative thumbnail path
// Returns empty string if thumbnail generation is skipped or fails
func processThumbnail(originalPath string, destFolder string) (string, error) {
	// Check if file is an image
	if !isImageFile(originalPath) {
		return "", nil
	}

	// Determine thumbnail directory (default: dest/.thumbnails)
	baseThumbDir := filepath.Join(destFolder, ".thumbnails")

	// Get relative path from destFolder to the original file
	relPath, err := filepath.Rel(destFolder, originalPath)
	if err != nil {
		// If we can't get relative path, use the filename
		relPath = filepath.Base(originalPath)
	}

	// Create thumbnail path maintaining directory structure
	thumbPath := filepath.Join(baseThumbDir, relPath)

	// Change extension to .jpg for thumbnails (or keep original if PNG)
	ext := filepath.Ext(thumbPath)
	if ext != ".png" {
		thumbPath = thumbPath[:len(thumbPath)-len(ext)] + ".jpg"
	}

	// Check if thumbnail already exists
	if _, err := os.Stat(thumbPath); err == nil {
		// Thumbnail exists, return relative path
		if relThumbPath, err := filepath.Rel(destFolder, thumbPath); err == nil {
			return filepath.ToSlash(relThumbPath), nil
		}
	}

	// Generate thumbnail (default size: 200px)
	if err := generateThumbnail(originalPath, thumbPath, 200); err != nil {
		return "", fmt.Errorf("thumbnail generation failed for %s: %w", filepath.Base(originalPath), err)
	}

	// Return relative path from destination folder
	if relThumbPath, err := filepath.Rel(destFolder, thumbPath); err == nil {
		return filepath.ToSlash(relThumbPath), nil
	}

	return "", nil
}
