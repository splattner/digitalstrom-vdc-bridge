# ── Stage 1: Build web UI ─────────────────────────────────────────────────────
FROM node:24-alpine AS web-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
# Output lands at /app/pkg/httpapi/webdist (vite outDir: ../pkg/httpapi/webdist)

# ── Stage 2: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.27-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
# Bring in the compiled web assets so go:embed can find them
COPY --from=web-builder /app/pkg/httpapi/webdist ./pkg/httpapi/webdist
COPY . .
ARG VERSION=dev
RUN go build \
    -ldflags="-s -w -X github.com/splattner/vdcgo/pkg/vdcgo.Version=${VERSION}" \
    -o /usr/local/bin/vdcgo-daemon \
    ./cmd/vdcgo-daemon

# ── Stage 3: Minimal runtime image ───────────────────────────────────────────
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-builder /usr/local/bin/vdcgo-daemon /usr/local/bin/vdcgo-daemon

VOLUME ["/data"]
EXPOSE 8090 8999

ENTRYPOINT ["vdcgo-daemon"]
CMD ["--non-local", "--http-listen", ":8090", "--datadir", "/data", "--listen", "8999"]
