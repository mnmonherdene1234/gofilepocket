package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type App struct {
	config Config
	store  *FileStore
	logger *log.Logger
}

type DeleteFileRequest struct {
	Filename string `json:"filename"`
}

type UploadResponse struct {
	Message     string `json:"message"`
	Filename    string `json:"filename"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}

type DeleteResponse struct {
	Message  string `json:"message"`
	Filename string `json:"filename"`
}

type FilesListResponse struct {
	Files []FileMeta `json:"files"`
}

type SizeResponse struct {
	Size int64 `json:"size"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewApp(cfg Config) *App {
	return NewAppWithLogger(cfg, log.Default())
}

func NewAppWithLogger(cfg Config, logger *log.Logger) *App {
	if logger == nil {
		logger = log.Default()
	}
	return &App{
		config: cfg,
		store:  NewFileStore(cfg.FilesDir),
		logger: logger,
	}
}

func (a *App) EnsureStorage() error {
	return a.store.EnsureWritable()
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	p := a.config.APIPrefix

	mux.Handle("GET "+p+"/{$}", a.public(http.HandlerFunc(a.handleIndex)))
	mux.Handle("GET "+p+"/health", a.public(http.HandlerFunc(a.handleHealth)))
	mux.Handle("POST "+p+"/upload", a.protected(http.HandlerFunc(a.handleUpload)))
	mux.Handle("DELETE "+p+"/delete", a.protected(http.HandlerFunc(a.handleDelete)))
	mux.Handle("GET "+p+"/list", a.protected(http.HandlerFunc(a.handleList)))
	mux.Handle("GET "+p+"/size", a.protected(http.HandlerFunc(a.handleSize)))

	if p != "" {
		mux.Handle(p, http.RedirectHandler(p+"/", http.StatusMovedPermanently))
	}

	if a.config.ServeStaticFiles {
		fullStatic := a.fullStaticPath()
		staticPrefix := fullStatic + "/"
		fileServer := http.StripPrefix(staticPrefix, http.FileServer(noDirListFS{root: http.Dir(a.config.FilesDir)}))
		mux.Handle(staticPrefix, a.public(fileServer))
		mux.Handle(fullStatic, a.public(http.RedirectHandler(staticPrefix, http.StatusMovedPermanently)))
	}

	return a.withCORS(a.withLogging(mux))
}

func (a *App) fullStaticPath() string {
	return a.config.APIPrefix + a.config.StaticFilesPath
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	p := a.config.APIPrefix
	a.writeJSON(w, http.StatusOK, map[string]any{
		"name": "FilePocket",
		"endpoints": []string{
			"GET " + p + "/health",
			"POST " + p + "/upload",
			"DELETE " + p + "/delete",
			"GET " + p + "/list",
			"GET " + p + "/size",
		},
		"static_files_path": a.fullStaticPath(),
	})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	if a.config.MaxUploadSizeMB > 0 {
		maxSize := a.config.MaxUploadSizeMB << 20
		r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	}
	if err := r.ParseMultipartForm(a.config.MaxUploadMemoryMB << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			a.writeError(w, http.StatusRequestEntityTooLarge, "Upload size exceeds limit", err)
			return
		}
		a.writeError(w, http.StatusBadRequest, "Invalid multipart form", err)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "No file received", err)
		return
	}
	defer file.Close()

	filename := SafeBaseName(header.Filename)
	if !strings.EqualFold(r.FormValue("useOriginalFilename"), "true") {
		filename = UniqueFilename(filename)
	}

	if err := a.store.Save(file, filename); err != nil {
		a.writeStoreError(w, "Failed to save the file", err)
		return
	}

	var downloadURL string
	if a.config.ServeStaticFiles {
		downloadURL = a.fullStaticPath() + "/" + url.PathEscape(filename)
	}

	a.writeJSON(w, http.StatusOK, UploadResponse{
		Message:     "File uploaded successfully",
		Filename:    filename,
		DownloadURL: downloadURL,
	})
}

func (a *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req DeleteFileRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	if strings.TrimSpace(req.Filename) == "" {
		a.writeError(w, http.StatusBadRequest, "Filename is required", errors.New("missing filename"))
		return
	}

	if err := a.store.Delete(req.Filename); err != nil {
		a.writeStoreError(w, "Failed to delete the file", err)
		return
	}

	a.writeJSON(w, http.StatusOK, DeleteResponse{
		Message:  "File deleted successfully",
		Filename: req.Filename,
	})
}

func (a *App) handleList(w http.ResponseWriter, r *http.Request) {
	files, err := a.store.List()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "Failed to list files", err)
		return
	}

	a.writeJSON(w, http.StatusOK, FilesListResponse{Files: files})
}

func (a *App) handleSize(w http.ResponseWriter, r *http.Request) {
	size, err := a.store.FolderSize()
	if err != nil {
		a.writeError(w, http.StatusInternalServerError, "Failed to calculate folder size", err)
		return
	}

	a.writeJSON(w, http.StatusOK, SizeResponse{Size: size})
}

func (a *App) public(next http.Handler) http.Handler {
	return next
}

type noDirListFS struct {
	root http.Dir
}

func (fs noDirListFS) Open(name string) (http.File, error) {
	f, err := fs.root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

func (a *App) protected(next http.Handler) http.Handler {
	if !a.config.APIKeyEnabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get(a.config.APIKeyHeader)
		if apiKey == "" {
			a.writeError(w, http.StatusUnauthorized, "API key is required", errors.New("missing api key"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(a.config.APIKey)) != 1 {
			a.writeError(w, http.StatusUnauthorized, "Invalid API key", errors.New("invalid api key"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) withCORS(next http.Handler) http.Handler {
	allowHeaders := "Content-Type, Accept"
	if a.config.APIKeyHeader != "" {
		allowHeaders += ", " + a.config.APIKeyHeader
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *App) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		a.logger.Printf("%s %s %d %s", r.Method, r.URL.Path, recorder.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (a *App) writeStoreError(w http.ResponseWriter, fallbackMessage string, err error) {
	switch {
	case errors.Is(err, ErrInvalidFilename):
		a.writeError(w, http.StatusBadRequest, "Invalid filename", err)
	case errors.Is(err, ErrFileNotFound):
		a.writeError(w, http.StatusNotFound, "File not found", err)
	case errors.Is(err, ErrFileAlreadyExists):
		a.writeError(w, http.StatusConflict, "File already exists", err)
	default:
		a.writeError(w, http.StatusInternalServerError, fallbackMessage, err)
	}
}

func (a *App) writeError(w http.ResponseWriter, status int, message string, err error) {
	if err != nil {
		a.logger.Printf("%s: %v", message, err)
	}
	a.writeJSON(w, status, ErrorResponse{Error: message})
}

func (a *App) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		a.logger.Printf("failed to encode response: %v", err)
	}
}
