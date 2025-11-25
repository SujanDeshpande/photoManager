package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// BuildMetadataJSON inspects the file; for images extracts EXIF, for videos extracts XMP (if found).
// Returns a JSON object string. Never returns empty string; defaults to "{}".
func BuildMetadataJSON(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if isImageExt(ext) {
		if ed, err := ExtractExif(path); err == nil && ed != nil {
			if b, mErr := json.Marshal(ed); mErr == nil {
				return string(b)
			}
		}
	} else if isVideoExt(ext) {
		if xmp := extractXMPPacket(path); xmp != "" {
			obj := map[string]string{
				"type": "xmp",
				"raw":  xmp,
			}
			if b, err := json.Marshal(obj); err == nil {
				return string(b)
			}
		}
		// Some videos may still have EXIF; attempt as fallback
		if ed, err := ExtractExif(path); err == nil && ed != nil {
			if b, mErr := json.Marshal(ed); mErr == nil {
				return string(b)
			}
		}
	}
	return "{}"
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".tif", ".tiff", ".heic", ".heif", ".nef", ".cr2", ".cr3", ".arw", ".png":
		return true
	default:
		return false
	}
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".m4v", ".avi", ".mkv", ".hevc":
		return true
	default:
		return false
	}
}

// extractXMPPacket makes a best-effort attempt to find an embedded XMP packet in the file.
// It scans the file for "<x:xmpmeta" ... "</x:xmpmeta>" and returns the raw XML if found.
func extractXMPPacket(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	startTag := []byte("<x:xmpmeta")
	endTag := []byte("</x:xmpmeta>")
	start := indexBytes(data, startTag)
	if start == -1 {
		return ""
	}
	end := indexBytes(data[start:], endTag)
	if end == -1 {
		return ""
	}
	end += start + len(endTag)
	return string(data[start:end])
}

// indexBytes returns the index of the first instance of sep in s, or -1 if sep is not present in s.
func indexBytes(s, sep []byte) int {
	// simple search
	n := len(sep)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i] == sep[0] && string(s[i:i+n]) == string(sep) {
			return i
		}
	}
	return -1
}
