# Build for current platform (for local testing)
.PHONY: build
build:
	go build -o mysql-health-check .

# Cross-compile for Linux AMD64 (most common servers)
.PHONY: build-linux-amd64
build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o mysql-health-check-linux-amd64 .
	@echo "Built mysql-health-check-linux-amd64 - copy to your server and chmod +x"

# Cross-compile for Linux ARM64 (e.g. Graviton, Raspberry Pi)
.PHONY: build-linux-arm64
build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o mysql-health-check-linux-arm64 .
	@echo "Built mysql-health-check-linux-arm64 - copy to your server and chmod +x"

# Build both Linux variants for releases
.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64
