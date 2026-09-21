package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestApp creates an App with a discarding logger to keep test output clean.
func newTestApp(cfg Config) *App {
	return NewAppWithLogger(cfg, log.New(io.Discard, "", 0))
}

func TestHandleUploadEscapesDownloadURL(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(Config{
		FilesDir:          dir,
		StaticFilesPath:   "/files",
		ServeStaticFiles:  true,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	req, err := newMultipartUploadRequest("report 2026 #1.txt", "hello", true)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp UploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Filename != "report 2026 #1.txt" {
		t.Fatalf("unexpected filename: %q", resp.Filename)
	}

	if resp.DownloadURL != "/files/report%202026%20%231.txt" {
		t.Fatalf("unexpected download URL: %q", resp.DownloadURL)
	}

	if _, err := os.Stat(filepath.Join(dir, "report 2026 #1.txt")); err != nil {
		t.Fatalf("expected uploaded file to exist: %v", err)
	}
}

func TestCORSIncludesConfiguredAPIKeyHeader(t *testing.T) {
	app := newTestApp(Config{
		APIKeyEnabled:    true,
		APIKeyHeader:     "X-Custom-Key",
		APIKey:           "secret",
		FilesDir:         t.TempDir(),
		ServeStaticFiles: false,
		StaticFilesPath:  "/files",
	})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}

	allowHeaders := rr.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "X-Custom-Key") {
		t.Fatalf("expected allow headers to include custom API key header, got %q", allowHeaders)
	}
}

func TestHandleIndex(t *testing.T) {
	app := newTestApp(Config{FilesDir: t.TempDir(), StaticFilesPath: "/files"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["name"] != "FilePocket" {
		t.Fatalf("unexpected name: %v", body["name"])
	}
}

func TestHandleHealth(t *testing.T) {
	app := newTestApp(Config{FilesDir: t.TempDir(), StaticFilesPath: "/files"})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected status: %v", body["status"])
	}
}

func TestHandleUploadNoFile(t *testing.T) {
	app := newTestApp(Config{
		FilesDir:          t.TempDir(),
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUploadUniqueFilename(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(Config{
		FilesDir:          dir,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	req, err := newMultipartUploadRequest("hello.txt", "content", false)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp UploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Filename == "hello.txt" {
		t.Fatal("expected unique filename, got original")
	}
	if !strings.HasPrefix(resp.Filename, "hello_") {
		t.Fatalf("expected unique filename to start with hello_, got %q", resp.Filename)
	}
}

func TestHandleUploadSizeLimit(t *testing.T) {
	app := newTestApp(Config{
		FilesDir:          t.TempDir(),
		MaxUploadMemoryMB: 1,
		MaxUploadSizeMB:   1,
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fw, err := writer.CreateFormFile("file", "big.bin")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	// Write 2 MB to exceed the 1 MB limit
	chunk := make([]byte, 1024)
	for i := 0; i < 2048; i++ {
		if _, err := fw.Write(chunk); err != nil {
			t.Fatalf("write chunk: %v", err)
		}
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUploadAllowsUnlimitedWhenSizeLimitIsZero(t *testing.T) {
	app := newTestApp(Config{
		FilesDir:          t.TempDir(),
		MaxUploadMemoryMB: 1,
		MaxUploadSizeMB:   0,
	})

	req, err := newMultipartUploadRequest("big.bin", strings.Repeat("x", 2*1024*1024), true)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleDeleteAndList(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(Config{
		FilesDir:          dir,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	// Upload a file first
	req, err := newMultipartUploadRequest("todelete.txt", "data", true)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", rr.Code, rr.Body.String())
	}

	// List – should contain the file
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/list", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list failed: %d", rr.Code)
	}
	var listResp FilesListResponse
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResp.Files) != 1 || listResp.Files[0].Name != "todelete.txt" {
		t.Fatalf("unexpected list: %+v", listResp.Files)
	}

	// Delete the file
	deleteBody := `{"filename":"todelete.txt"}`
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/delete", strings.NewReader(deleteBody)))
	if rr.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", rr.Code, rr.Body.String())
	}
	var delResp DeleteResponse
	if err := json.NewDecoder(rr.Body).Decode(&delResp); err != nil {
		t.Fatalf("decode delete: %v", err)
	}
	if delResp.Filename != "todelete.txt" {
		t.Fatalf("unexpected delete filename: %q", delResp.Filename)
	}

	// File must be gone from disk
	if _, err := os.Stat(filepath.Join(dir, "todelete.txt")); !os.IsNotExist(err) {
		t.Fatal("expected file to be deleted from disk")
	}

	// Delete again – should return 404
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/delete", strings.NewReader(deleteBody)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on re-delete, got %d", rr.Code)
	}
}

func TestHandleDeleteMissingFilename(t *testing.T) {
	app := newTestApp(Config{FilesDir: t.TempDir()})
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/delete", strings.NewReader(`{"filename":""}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleDeleteInvalidJSON(t *testing.T) {
	app := newTestApp(Config{FilesDir: t.TempDir()})
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/delete", strings.NewReader(`not json`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleSize(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(Config{
		FilesDir:          dir,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	content := "hello world"
	req, err := newMultipartUploadRequest("sized.txt", content, true)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload failed: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/size", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("size failed: %d", rr.Code)
	}
	var sizeResp SizeResponse
	if err := json.NewDecoder(rr.Body).Decode(&sizeResp); err != nil {
		t.Fatalf("decode size: %v", err)
	}
	if sizeResp.Size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), sizeResp.Size)
	}
}

func TestProtectedEndpointRequiresAPIKey(t *testing.T) {
	app := newTestApp(Config{
		APIKeyEnabled:     true,
		APIKeyHeader:      "X-API-Key",
		APIKey:            "secret",
		FilesDir:          t.TempDir(),
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	// No API key → 401
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/list", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	// Wrong API key → 401
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	req.Header.Set("X-API-Key", "wrong")
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", rr.Code)
	}

	// Correct API key → 200
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/list", nil)
	req.Header.Set("X-API-Key", "secret")
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct key, got %d", rr.Code)
	}
}

func TestHandleUploadConflictWithOriginalFilename(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(Config{
		FilesDir:          dir,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	for i, expectCode := range []int{http.StatusOK, http.StatusConflict} {
		req, err := newMultipartUploadRequest("dup.txt", "data", true)
		if err != nil {
			t.Fatalf("build request %d: %v", i, err)
		}
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		if rr.Code != expectCode {
			t.Fatalf("attempt %d: expected %d, got %d: %s", i+1, expectCode, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleUploadNoDownloadURLWhenStaticDisabled(t *testing.T) {
	app := newTestApp(Config{
		FilesDir:          t.TempDir(),
		ServeStaticFiles:  false,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})

	req, err := newMultipartUploadRequest("nodl.txt", "hi", true)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp UploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DownloadURL != "" {
		t.Fatalf("expected empty downloadUrl, got %q", resp.DownloadURL)
	}
}

func TestSavePartialCleanupOnCopyError(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	// A reader that returns an error after yielding some bytes
	errReader := io.MultiReader(strings.NewReader("partial"), &errorReader{})

	err := store.Save(errReader, "partial.txt")
	if err == nil {
		t.Fatal("expected error from Save")
	}

	// The partial file must not remain on disk
	if _, statErr := os.Stat(filepath.Join(dir, "partial.txt")); !os.IsNotExist(statErr) {
		t.Fatal("expected partial file to be cleaned up after copy error")
	}
}

func TestStaticFilesArePublicWhenAPIKeyEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "public.txt"), []byte("public content"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	app := newTestApp(Config{
		APIKeyEnabled:    true,
		APIKeyHeader:     "X-API-Key",
		APIKey:           "secret",
		FilesDir:         dir,
		StaticFilesPath:  "/files",
		ServeStaticFiles: true,
	})

	// Static file access without an API key must succeed.
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/files/public.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 without API key, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "public content" {
		t.Fatalf("expected body %q, got %q", "public content", got)
	}

	// The extensionless /files redirect must be public too.
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/files", nil))
	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 without API key, got %d", rr.Code)
	}
}

func TestAPIPrefixRouting(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(Config{
		FilesDir:          dir,
		APIPrefix:         "/api/v1",
		StaticFilesPath:   "/files",
		ServeStaticFiles:  true,
		MaxUploadMemoryMB: 32,
		MaxUploadSizeMB:   100,
	})
	h := app.Handler()

	// Health under the prefix.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed health, got %d", rr.Code)
	}

	// Old unprefixed paths must not serve.
	for _, target := range []string{"/health", "/list", "/size"} {
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for legacy path %s, got %d", target, rr.Code)
		}
	}

	// Upload under the prefix.
	req, err := newMultipartUploadRequest("prefixed.txt", "data", true)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.URL.Path = "/api/v1/upload"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed upload, got %d: %s", rr.Code, rr.Body.String())
	}
	var uploadResp UploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	if uploadResp.DownloadURL != "/api/v1/files/prefixed.txt" {
		t.Fatalf("unexpected prefixed download URL: %q", uploadResp.DownloadURL)
	}

	// List and size under the prefix.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/list", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed list, got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/size", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed size, got %d", rr.Code)
	}

	// Static file under the prefix; directory listing stays disabled.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/files/prefixed.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed static file, got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/files/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for prefixed directory listing, got %d", rr.Code)
	}

	// Index reports prefixed endpoints and static path.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed index, got %d", rr.Code)
	}
	var index map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&index); err != nil {
		t.Fatalf("decode index: %v", err)
	}
	if index["static_files_path"] != "/api/v1/files" {
		t.Fatalf("unexpected static_files_path: %v", index["static_files_path"])
	}
	found := false
	for _, e := range index["endpoints"].([]any) {
		if e.(string) == "POST /api/v1/upload" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected prefixed endpoints in index, got %v", index["endpoints"])
	}

	// Delete under the prefix.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/delete", strings.NewReader(`{"filename":"prefixed.txt"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed delete, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStaticDirListingDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret1.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	app := newTestApp(Config{
		FilesDir:         dir,
		StaticFilesPath:  "/files",
		ServeStaticFiles: true,
	})

	// Directory listing must not expose filenames.
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/files/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for directory listing, got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "secret1.txt") {
		t.Fatal("directory listing exposed filenames")
	}

	// Exact file paths must keep working.
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/files/secret1.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for exact file, got %d", rr.Code)
	}
}

// errorReader always returns an error on Read.
type errorReader struct{}

func (*errorReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func newMultipartUploadRequest(filename, contents string, useOriginalFilename bool) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := fileWriter.Write([]byte(contents)); err != nil {
		return nil, err
	}

	if useOriginalFilename {
		if err := writer.WriteField("useOriginalFilename", "true"); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}
