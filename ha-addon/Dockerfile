# Must be declared before any FROM to be
# usable in a FROM instruction.
ARG BUILD_FROM=ghcr.io/home-assistant/base:latest

# ── Stage 1: Build web UI ─────────────────────────────────────────────────────
FROM node:24-alpine AS web-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
# vite outDir is ../pkg/httpapi/webdist → output lands at /app/pkg/httpapi/webdist

# ── Stage 2: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.27-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY --from=web-builder /app/pkg/httpapi/webdist ./pkg/httpapi/webdist
COPY . .
ARG VERSION=dev
RUN go build \
    -ldflags="-s -w -X github.com/splattner/vdcgo/pkg/vdcgo.Version=${VERSION}" \
    -o /usr/local/bin/vdcgo-daemon \
    ./cmd/vdcgo-daemon

# ── Stage 3: Home Assistant base image ───────────────────────────────────────
FROM ${BUILD_FROM}

COPY --from=go-builder /usr/local/bin/vdcgo-daemon /usr/local/bin/vdcgo-daemon
COPY ha-addon/run.sh /run.sh
RUN chmod +x /run.sh

CMD ["/run.sh"]
