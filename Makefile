.PHONY: all build web web-dev test clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/splattner/vdcgo/pkg/vdcgo.Version=$(VERSION)

all: build

# Build the frontend then the Go binary
build: web
	go build -ldflags "$(LDFLAGS)" ./...

# Build the frontend (outputs to pkg/httpapi/webdist)
web:
	cd web && npm run build

# Start the Vite dev server (proxies /api → :8090)
web-dev:
	cd web && npm run dev

# Run all Go tests
test:
	go test ./...

clean:
	rm -rf web/node_modules pkg/httpapi/webdist/assets
