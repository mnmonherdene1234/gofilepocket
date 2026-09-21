# FilePocket API Reference

## Base Prefix

All paths below are shown without a prefix. When `API_PREFIX` is set
(e.g. `API_PREFIX=/api/v1`), prepend it to every endpoint, including
static files: `GET /api/v1/health`, `POST /api/v1/upload`,
`GET /api/v1/files/{path}`.

## Authentication

When `API_KEY_ENABLED=true`, include the configured API key header on these endpoints:

- `POST /upload`
- `DELETE /delete`
- `GET /list`
- `GET /size`

Public endpoints (no API key needed, even when `API_KEY_ENABLED=true`):

- `GET /`
- `GET /health`
- static file access (`GET /files/{path}`) — see below, this is
  intentionally public so shared `downloadUrl` links work without a key

## GET /

Returns basic service info.

Response:

```json
{
  "name": "FilePocket",
  "endpoints": [
    "GET /health",
    "POST /upload",
    "DELETE /delete",
    "GET /list",
    "GET /size"
  ],
  "static_files_path": "/files"
}
```

## GET /health

Simple health check.

Response:

```json
{ "status": "ok" }
```

## POST /upload

Upload a file using `multipart/form-data`.

Form fields:

- `file` required
- `useOriginalFilename` optional; set to `true` to keep the original file name

Response:

```json
{
  "message": "File uploaded successfully",
  "filename": "example.txt",
  "downloadUrl": "/files/example.txt"
}
```

Notes:

- `downloadUrl` is only returned when static file serving is enabled.
- `downloadUrl` is URL-encoded when the filename contains reserved characters or spaces.
- If `useOriginalFilename=true` and the file already exists, the API returns `409 Conflict`.
- Uploads are unlimited by default. When `MAX_UPLOAD_SIZE_MB` is set to a positive number, larger uploads return `413 Payload Too Large`.

## DELETE /delete

Delete a file by name.

Request:

```json
{ "filename": "example.txt" }
```

Response:

```json
{
  "message": "File deleted successfully",
  "filename": "example.txt"
}
```

## GET /list

List stored files.

Response:

```json
{
  "files": [{ "name": "example.txt", "size": 1234 }]
}
```

## GET /size

Return the total size of stored files.

Response:

```json
{ "size": 1234 }
```

## GET /files/{path}

Serve stored files statically when `IS_SERVE_STATIC_FILES=true`.

> **Public by design.** This endpoint never requires an API key, even
> when `API_KEY_ENABLED=true`. Anyone who knows (or guesses) the exact
> filename can download it, e.g. `GET /files/report.txt`. Keep this in
> mind for sensitive files: prefer non-guessable names (omit
> `useOriginalFilename` so the server generates a unique name) or set
> `IS_SERVE_STATIC_FILES=false` to disable static serving entirely and
> manage access yourself.
>
> Directory listing is disabled: requesting the directory itself
> (`GET /files/`) returns `404`, so filenames cannot be enumerated.
> Only exact file paths are served.

The actual prefix is configured by `STATIC_FILES_SERVE_PATH`.

## Errors

API endpoints return JSON errors:

```json
{ "error": "message" }
```

Static file requests are served by `http.FileServer`, so missing files may
return the Go server's default 404 response instead of JSON.

Common status codes:

- `400` bad request
- `401` missing or invalid API key
- `404` file not found
- `409` file already exists
- `413` upload too large
- `500` internal server error

## CORS

Responses include:

```text
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type, Accept, <configured API key header>
```

Preflight `OPTIONS` requests return `204 No Content`.
