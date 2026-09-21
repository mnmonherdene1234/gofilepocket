# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gofilepocket .

FROM alpine:3.21

RUN addgroup -S -g 1000 app \
    && adduser -S -u 1000 -G app app \
    && mkdir -p /app/files \
    && chown -R app:app /app

WORKDIR /app

COPY --from=build /out/gofilepocket /usr/local/bin/gofilepocket

ENV SERVER_PORT=9935 \
    FILES_DIR=/app/files \
    STATIC_FILES_SERVE_PATH=/files \
    IS_SERVE_STATIC_FILES=true

EXPOSE 9935

USER app

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:${SERVER_PORT}${API_PREFIX:-}/health >/dev/null 2>&1 || exit 1

CMD ["/usr/local/bin/gofilepocket"]
