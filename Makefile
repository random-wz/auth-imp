.PHONY: help build run test clean install

# 默认目标
help:
	@echo "Available targets:"
	@echo "  build       - Build server and CLI"
	@echo "  run         - Run IDP server"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  install     - Install idpctl to /usr/local/bin"

# 构建
build:
	@echo "Building IDP server..."
	@go build -o bin/idp-server ./cmd
	@echo "Building idpctl CLI..."
	@go build -o idpctl ./cmd/idpctl
	@echo "Build complete!"

# 运行服务器
run: build
	@echo "Starting IDP server..."
	@./bin/idp-server

# 测试
test:
	@echo "Running tests..."
	@go test -v ./...

# 清理
clean:
	@echo "Cleaning..."
	@rm -rf bin/ idpctl
	@echo "Clean complete!"

# 安装 CLI
install: build
	@echo "Installing idpctl..."
	@cp idpctl /usr/local/bin/
	@echo "idpctl installed to /usr/local/bin/"
