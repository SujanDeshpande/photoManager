package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type apiError struct {
	Error string `json:"error"`
}

type healthResp struct {
	Ok        bool      `json:"ok"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
}

type scanReq struct {
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

type scanResp struct {
	Started bool   `json:"started"`
	Status  string `json:"status"`
	Copied  int64  `json:"copied"`
	Skipped int64  `json:"skipped"`
	Failed  int64  `json:"failed"`
}

type updateTagsReq struct {
	Tags []string `json:"tags"`
}

type updateTagsResp struct {
	Ok   bool     `json:"ok"`
	Tags []string `json:"tags"`
}

func StartServer(addr string, dbFile string) error {
	r := mux.NewRouter()
	r.HandleFunc("/api/health", handleHealth).Methods(http.MethodGet)
	r.HandleFunc("/api/incoming", withDB(dbFile, handleListIncoming)).Methods(http.MethodGet)
	r.HandleFunc("/api/outcoming", withDB(dbFile, handleListOutcoming)).Methods(http.MethodGet)
	r.HandleFunc("/api/outcoming/{id}", withDB(dbFile, handleGetOutcoming)).Methods(http.MethodGet)
	r.HandleFunc("/api/outcoming/{id}/tags", withDB(dbFile, handleTags)).Methods(http.MethodPost)
	r.HandleFunc("/api/scan", withDB(dbFile, handleScan)).Methods(http.MethodPost)
	r.HandleFunc("/api/scan/status", handleScanStatus).Methods(http.MethodGet)
	r.HandleFunc("/api/clear", withDB(dbFile, func(w http.ResponseWriter, r *http.Request, db *DB) {
		if err := db.clearDBTables(); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	})).Methods(http.MethodPost)
	// Serve thumbnails
	r.HandleFunc("/api/thumbnails/{path:.*}", func(w http.ResponseWriter, r *http.Request) {
		handleThumbnail(w, r, dbFile)
	}).Methods(http.MethodGet)
	// Serve files
	r.HandleFunc("/api/files/{path:.*}", func(w http.ResponseWriter, r *http.Request) {
		handleFile(w, r, dbFile)
	}).Methods(http.MethodGet)

	cors := handlers.CORS(
		handlers.AllowedHeaders([]string{"Content-Type", "Accept"}),
		handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"}),
	)

	srv := &http.Server{
		Addr:              addr,
		Handler:           cors(r),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Println("Serving HTTP API on", addr)
	return srv.ListenAndServe()
}

func handleScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, GetScanStatus())
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResp{Ok: true, Version: "0.1.0", Timestamp: time.Now()})
}

func withDB(dbFile string, next func(http.ResponseWriter, *http.Request, *DB)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, err := openAndInitDB(dbFile)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
			return
		}
		defer db.Close()
		next(w, r, db)
	}
}

func handleListIncoming(w http.ResponseWriter, r *http.Request, db *DB) {
	offset, limit := parsePage(r)
	rows, err := db.listIncomingRows(offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func handleListOutcoming(w http.ResponseWriter, r *http.Request, db *DB) {
	offset, limit := parsePage(r)
	rows, err := db.listOutcomingRows(offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func handleGetOutcoming(w http.ResponseWriter, r *http.Request, db *DB) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid id"})
		return
	}
	row, err := db.getOutcomingByIDRow(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "not found"})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func handleTags(w http.ResponseWriter, r *http.Request, db *DB) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid id"})
		return
	}

	var req updateTagsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid request body"})
		return
	}

	// Validate tags array (can be empty, but must be an array)
	if req.Tags == nil {
		req.Tags = []string{}
	}

	// Update tags in database
	if err := db.updateTags(id, req.Tags); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updateTagsResp{
		Ok:   true,
		Tags: req.Tags,
	})
}

func handleScan(w http.ResponseWriter, r *http.Request, db *DB) {
	var req scanReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	config := ProcessingConfig{
		SrcFolder:  req.Src,
		DestFolder: req.Dest,
	}

	// Start file processing in background (returns immediately)
	fileProcessing(config)

	resp := scanResp{
		Started: true,
		Status:  "started",
		Skipped: 0,
		Failed:  0,
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func parsePage(r *http.Request) (int64, int64) {
	q := r.URL.Query()
	var (
		offset int64 = 0
		limit  int64 = 50
	)
	if s := q.Get("offset"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v >= 0 {
			offset = v
		}
	}
	if s := q.Get("limit"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	return offset, limit
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func handleThumbnail(w http.ResponseWriter, r *http.Request, dbFile string) {
	// Get the thumbnail path from URL
	thumbPath := mux.Vars(r)["path"]
	if thumbPath == "" {
		http.Error(w, "thumbnail path required", http.StatusBadRequest)
		return
	}

	// Determine base dest folder from dbFile path
	baseDestFolder := filepath.Dir(dbFile)

	// Construct full thumbnail path
	// The path comes URL-encoded, so we need to decode it
	thumbPath = strings.ReplaceAll(thumbPath, "%2F", "/")
	// Convert from URL path format to filepath format
	thumbPath = filepath.FromSlash(thumbPath)
	fullThumbPath := filepath.Join(baseDestFolder, thumbPath)

	// Security: ensure the path is within the thumbnails directory
	absThumbDir, err := filepath.Abs(filepath.Join(baseDestFolder, ".thumbnails"))
	if err != nil {
		http.Error(w, "invalid thumbnail directory", http.StatusInternalServerError)
		return
	}
	absThumbPath, err := filepath.Abs(fullThumbPath)
	if err != nil {
		http.Error(w, "invalid thumbnail path", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(absThumbPath, absThumbDir) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	// Check if file exists
	if _, err := os.Stat(absThumbPath); os.IsNotExist(err) {
		http.Error(w, "thumbnail not found", http.StatusNotFound)
		return
	}

	// Determine content type based on file extension
	ext := strings.ToLower(filepath.Ext(absThumbPath))
	contentType := "image/jpeg" // default
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	}

	// Serve the file
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	http.ServeFile(w, r, absThumbPath)
}

func handleFile(w http.ResponseWriter, r *http.Request, dbFile string) {
	// Get the file path from URL
	filePath := mux.Vars(r)["path"]
	if filePath == "" {
		http.Error(w, "file path required", http.StatusBadRequest)
		return
	}

	// Determine base dest folder from dbFile path
	baseDestFolder := filepath.Dir(dbFile)

	// Construct full file path
	// The path comes URL-encoded, decode it
	decodedPath, err := url.PathUnescape(filePath)
	if err != nil {
		// Fallback: try QueryUnescape for better handling
		decodedPath, err = url.QueryUnescape(filePath)
		if err != nil {
			// Last resort: simple replacement
			decodedPath = strings.ReplaceAll(filePath, "%2F", "/")
			decodedPath = strings.ReplaceAll(decodedPath, "%20", " ")
		}
	}
	// Convert from URL path format to filepath format
	decodedPath = filepath.FromSlash(decodedPath)

	// Check if the path is already absolute (starts with / on Unix or drive letter on Windows)
	var fullFilePath string
	if filepath.IsAbs(decodedPath) {
		// Path is already absolute, use it directly
		fullFilePath = decodedPath
	} else {
		// Path is relative - try to reconstruct absolute path
		// First, try joining with base dest folder (for truly relative paths)
		relativePath := filepath.Join(baseDestFolder, decodedPath)

		// Check if this creates a valid path that exists
		if _, err := os.Stat(relativePath); err == nil {
			// File exists at relative path, use it
			fullFilePath = relativePath
		} else {
			// File doesn't exist at relative path, try adding leading slash
			// This handles cases where the leading slash was removed from an absolute path
			absolutePath := filepath.Join("/", decodedPath)
			if _, err := os.Stat(absolutePath); err == nil {
				// File exists at absolute path, use it
				fullFilePath = absolutePath
			} else {
				// Neither exists, use relative path (will fail later with better error message)
				fullFilePath = relativePath
			}
		}
	}

	// Security: ensure the path is within the dest folder
	absDestDir, err := filepath.Abs(baseDestFolder)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid destination directory: %v", err), http.StatusInternalServerError)
		return
	}
	absFilePath, err := filepath.Abs(fullFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid file path: %v", err), http.StatusBadRequest)
		return
	}

	// Check if the file path is within the dest directory
	if !strings.HasPrefix(absFilePath, absDestDir) {
		http.Error(w, fmt.Sprintf("access denied: file path %s is not within %s", absFilePath, absDestDir), http.StatusForbidden)
		return
	}

	// Check if file exists
	if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("file not found: %s", absFilePath), http.StatusNotFound)
		return
	}

	// Set appropriate content type based on file extension
	ext := strings.ToLower(filepath.Ext(absFilePath))
	switch ext {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".mov":
		w.Header().Set("Content-Type", "video/quicktime")
	case ".avi":
		w.Header().Set("Content-Type", "video/x-msvideo")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	http.ServeFile(w, r, absFilePath)
}
