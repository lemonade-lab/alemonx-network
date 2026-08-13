SHELL := /bin/bash

BINARIES := \
	dist/alemonx-network-linux-amd64 \
	dist/alemonx-network-windows-amd64.exe \
	dist/alemonx-network-darwin-arm64 \
	dist/alemonx-network-darwin-amd64

.PHONY: test vet validate web build dist check

test:
	go test ./...

vet:
	go vet ./...

validate:
	go run ./cmd/alxman validate alx.json

# Build the plugin web UI (React + Tailwind, alx design tokens) into ../web.
web:
	cd frontend && yarn install --non-interactive && yarn build

build: $(BINARIES)

dist/alemonx-network-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

dist/alemonx-network-windows-amd64.exe:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

dist/alemonx-network-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

dist/alemonx-network-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

# Package each platform as a zip containing the full plugin directory
# (alx.json + dist/ + web/), matching the CI release artifact layout.
dist: build web
	mkdir -p release
	for target in linux-amd64 windows-amd64 darwin-arm64 darwin-amd64; do \
		case "$$target" in \
			windows-amd64) binary="dist/alemonx-network-windows-amd64.exe" ;; \
			*) binary="dist/alemonx-network-$$target" ;; \
		esac; \
		stage="release/alemonx-network-$$target/alemonx-network"; \
		mkdir -p "$$stage/dist"; \
		cp alx.json "$$stage/alx.json"; \
		cp -r web "$$stage/web"; \
		cp "$$binary" "$$stage/dist/"; \
		(cd "release/alemonx-network-$$target" && zip -qr "../alemonx-network-$$target.zip" alemonx-network); \
		rm -rf "release/alemonx-network-$$target"; \
	done
	@ls -la release/

check: test vet validate
