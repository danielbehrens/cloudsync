.PHONY: build test lint clean install run help

# Binary name
BINARY_NAME=cloudsync
BINARY_PATH=bin/$(BINARY_NAME)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	$(GOBUILD) -o $(BINARY_PATH) ./cmd/cloudsync

# Build for Windows
build-windows:
	@echo "Building $(BINARY_NAME) for Windows..."
	@mkdir -p bin
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o bin/$(BINARY_NAME).exe ./cmd/cloudsync

# Build for Linux
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o bin/$(BINARY_NAME)-linux ./cmd/cloudsync

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Vet code
vet:
	@echo "Vetting code..."
	$(GOVET) ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	$(GOCMD) clean

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Install binary to $GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BINARY_PATH) $(GOPATH)/bin/

# Run the application (with default local MinIO settings)
run:
	$(GOBUILD) -o $(BINARY_PATH) ./cmd/cloudsync
	$(BINARY_PATH) \
		-cloud-endpoint "localhost:9000" \
		-access-key "minioadmin" \
		-secret-key "minioadmin"

# Display help
help:
	@echo "Available targets:"
	@echo "  build          - Build the application"
	@echo "  build-windows  - Build for Windows"
	@echo "  build-linux    - Build for Linux"
	@echo "  test           - Run tests"
	@echo "  lint           - Run linter (requires golangci-lint)"
	@echo "  fmt            - Format code"
	@echo "  vet            - Vet code"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  install        - Install binary to GOPATH/bin"
	@echo "  run            - Build and run with default settings"
	@echo "  help           - Display this help message"
