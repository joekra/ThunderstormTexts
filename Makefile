.PHONY: all build run-monitor run-notifier clean test tidy

# Binary names
MONITOR_BINARY=monitor
NOTIFIER_BINARY=notifier
BUILD_DIR=bin

all: build

build:
	@echo "Building binaries..."
	@mkdir -p $(BUILD_DIR)
	cd go_app && go build -o ../$(BUILD_DIR)/$(MONITOR_BINARY) ./cmd/monitor
	cd go_app && go build -o ../$(BUILD_DIR)/$(NOTIFIER_BINARY) ./cmd/notifier
	@echo "Build complete. Binaries are in $(BUILD_DIR)/"

run-monitor:
	@echo "Running Monitor..."
	cd go_app && go run ./cmd/monitor

run-notifier:
	@echo "Running Notifier..."
	cd go_app && go run ./cmd/notifier

clean:
	@echo "Cleaning build directory..."
	rm -rf $(BUILD_DIR)
	@echo "Clean complete."

test:
	@echo "Running tests..."
	cd go_app && go test ./...

tidy:
	@echo "Tidying dependencies..."
	cd go_app && go mod tidy
