# FilePocket

Simple file storage API written in Go. Upload files via HTTP, list them,
check total size, download or delete them, and optionally serve them as
static files.

Features:

- `POST /upload` with `multipart/form-data`
- `GET /list`, `GET /size`, `DELETE /delete`
- Static file serving (e.g. `GET /files/example.txt`)
- Optional API key protection
- Optional upload size limit
- Single binary + Docker image, files persisted on disk

## Prerequisites

- Docker and Docker Compose (for the recommended setup), or
- Go 1.26+ (for running from source)

## Quick Start (Docker Compose)

1. Create the storage directory and fix ownership on Linux:

```bash
mkdir -p files
sudo chown -R $(id -u):$(id -g) files
```

2. Start the service:

```bash
UID=$(id -u) GID=$(id -g) docker compose up -d --build
```

On Windows/macOS the `chown` and `UID=/GID=` parts are unnecessary;
`docker compose up -d --build` is enough.

3. Check it is running:

```bash
curl http://localhost:9935/health
# {"status":"ok"}
```

Uploaded files are stored in `./files` next to your
`docker-compose.yml` and survive container restarts and recreates.

Stop the service:

```bash
docker compose down
```

Rebuild after code changes:

```bash
UID=$(id -u) GID=$(id -g) docker compose up -d --build
```

## Try It

```bash
# Upload (keeps the original filename)
curl -F "file=@example.txt" -F "useOriginalFilename=true" http://localhost:9935/upload

# Upload with a unique generated filename (default when useOriginalFilename is omitted)
curl -F "file=@example.txt" http://localhost:9935/upload

# List files
curl http://localhost:9935/list

# Download
curl -O http://localhost:9935/files/example.txt
```

If static serving is enabled (default), uploads also return a `downloadUrl`
such as `/files/example.txt`.

## Configuration

Settings come from environment variables or a `.env` file in the project
root. Copy `.env.example` to `.env` and edit it before starting Compose.

| Variable                  | Default     | Description                                                  |
| ------------------------- | ----------- | ------------------------------------------------------------ |
| `SERVER_PORT`             | `9935`      | HTTP server port (host and container)                        |
| `FILES_DIR`               | `./files`   | Local directory used to store files (`/app/files` in Docker) |
| `STATIC_FILES_SERVE_PATH` | `/files`    | Public path for static file serving                          |
| `IS_SERVE_STATIC_FILES`   | `true`      | Enable or disable static file serving                        |
| `API_KEY_ENABLED`         | `false`     | Require an API key for protected endpoints                   |
| `API_KEY_HEADER`          | `X-API-Key` | Request header name for the API key                          |
| `API_KEY`                 | empty       | API key value used when auth is enabled                      |
| `MAX_UPLOAD_MEMORY_MB`    | `32`        | Memory threshold for multipart parsing                |
| `MAX_UPLOAD_SIZE_MB`      | `0`         | Upload size limit in MB. `0` means unlimited          |
| `API_PREFIX`              | empty       | Prefix for all endpoints, e.g. `/api/v1`. Empty means no prefix |

Notes:

- When `API_KEY_ENABLED=true`, `API_KEY` must be set.
- `MAX_UPLOAD_SIZE_MB=0` means unlimited. Set a positive number only to
  enforce a limit (larger uploads return `413`).
- In Docker, `FILES_DIR` is `/app/files` inside the container and
  `./files` on the host. Change both the variable and the volume mount
  together if you customize the path.

### Endpoint Prefix

Set `API_PREFIX` to serve everything under a base path:

```bash
API_PREFIX=/api/v1
```

Then `POST /upload` becomes `POST /api/v1/upload`,
`GET /health` becomes `GET /api/v1/health`, and static files move to
`/api/v1/files/*`. The `downloadUrl` values returned by uploads include
the prefix automatically. Empty (default) keeps the current paths, so
existing setups are unaffected.

## API Key Protection

Enable it via environment:

```yaml
environment:
  API_KEY_ENABLED: "true"
  API_KEY_HEADER: X-API-Key
  API_KEY: change-this-secret
```

Then send the header with every protected request:

```bash
curl -H "X-API-Key: change-this-secret" http://localhost:9935/list
```

Protected: `POST /upload`, `DELETE /delete`, `GET /list`, `GET /size`.
Public: `GET /`, `GET /health`, and static file access (`GET /files/{path}`).

> **Note:** `/files/*` is intentionally public even when API key
> protection is on, so shared `downloadUrl` links work without a key.
> Anyone with the exact filename can download it, but the file list
> itself is not exposed (`GET /files/` returns `404`). For sensitive
> files use non-guessable names (upload without `useOriginalFilename`)
> or disable static serving with `IS_SERVE_STATIC_FILES=false`.

## Run From Source

```bash
cp .env.example .env
go run .
```

The server listens on `SERVER_PORT` (default `9935`).
Run tests with:

```bash
go test ./...
```

## Run With Plain Docker

```bash
mkdir -p files
sudo chown -R $(id -u):$(id -g) files
docker build -t gofilepocket .
docker run --rm -p 9935:9935 --user $(id -u):$(id -g) -v "$(pwd)/files:/app/files" gofilepocket
```

## Troubleshooting `permission denied` on Upload (Linux)

Symptom: upload fails and logs show
`open /app/files/...: permission denied`.

Cause: with a bind mount, the host directory ownership wins. If `./files`
is owned by `root` or another user, the container user (`app`, UID 1000)
cannot write to it. This appears on Linux but is often hidden on
Windows/macOS.

Diagnose:

```bash
ls -ldn ./files
id
docker exec gofilepocket id
docker logs gofilepocket --tail 50
```

Fix (do not use `chmod 777`):

```bash
mkdir -p files
sudo chown -R $(id -u):$(id -g) files
chmod 755 files
UID=$(id -u) GID=$(id -g) docker compose up -d --force-recreate
```

The app also checks `FILES_DIR` writability at startup and exits with a
clear error instead of failing only on the first upload.

## API Reference

See [API.md](API.md) for endpoints, request/response examples, status
codes, and CORS behavior.
