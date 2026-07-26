.PHONY: build frontend dev clean

# Build the frontend and embed it in the Go binary
build: frontend
	go build -o dist/tmuxatlas .

# Build just the frontend
frontend:
	cd web && npm install && npm run build

# Development mode - run Go server with live reload
dev:
	go run . server

# Clean build artifacts
clean:
	rm -rf dist/
	rm -rf web/dist/
	rm -rf web/node_modules/
	rm -rf pkg/server/dist/*
	touch pkg/server/dist/.gitkeep
