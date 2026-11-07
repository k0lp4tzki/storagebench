# Multi-stage build for the storagebench controller with the web UI embedded.
#
# Stage 1 builds the React/Vite frontend into cmd/controller/webdist.
# Stage 2 compiles the Go controller with CGO (required by the sqlite3 driver)
#         and the `embedui` build tag so the frontend is baked into the binary.
# Stage 3 is a small Debian runtime image (glibc, needed by CGO/sqlite3).
#
# Build from the repo root:
#   docker build -t storagebench-controller .
# Or via docker compose (see docker-compose.yml).

# ---- Stage 1: frontend ----
FROM node:20-bookworm-slim AS web
WORKDIR /src/web
# Copy manifests first for better layer caching.
COPY web/package.json web/package-lock.json* ./
# npm install resolves even when package.json changed vs. the committed lockfile.
RUN npm install
# Copy the rest of the frontend and build. Vite outputs to ../cmd/controller/webdist.
COPY web/ ./
# The build output dir lives outside web/, so make sure it exists.
RUN mkdir -p /src/cmd/controller/webdist && npm run build

# ---- Stage 2: Go build ----
FROM golang:1.22-bookworm AS build
# gcc/libc headers are present in the golang bookworm image; CGO needs them.
ENV CGO_ENABLED=1 GOOS=linux
WORKDIR /src
# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download
# Copy the full source.
COPY . .
# Bring in the built frontend from stage 1 (overwrites the placeholder webdist).
COPY --from=web /src/cmd/controller/webdist ./cmd/controller/webdist
# Build the controller with the embedded UI.
RUN go build -trimpath -tags embedui \
      -ldflags "-s -w \
        -X 'github.com/storagebench/storagebench/internal/version.Version=docker' \
        -X 'github.com/storagebench/storagebench/internal/version.BuildDate=unknown'" \
      -o /out/storagebench-controller ./cmd/controller

# ---- Stage 3: runtime ----
FROM debian:bookworm-slim AS runtime
# ca-certificates for any outbound TLS; create a non-root user.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /app --shell /usr/sbin/nologin appuser
WORKDIR /app
COPY --from=build /out/storagebench-controller /usr/local/bin/storagebench-controller
# Data directory for the SQLite database (mounted as a volume in compose).
RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser
# Plaintext HTTP by default inside the container; terminate TLS in front of it
# or set STORAGEBENCH_LISTEN_ADDR and mount certs for direct TLS.
ENV STORAGEBENCH_LISTEN_ADDR=0.0.0.0:8080 \
    STORAGEBENCH_DB_PATH=/app/data/storagebench.db
EXPOSE 8080
ENTRYPOINT ["storagebench-controller"]
