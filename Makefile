# Apexy API Gateway - 常用开发命令
BINARY  := apexy
PKG     := ./cmd/gateway
IMAGE   := apexy:latest
GO      ?= go

.PHONY: all build run test race vet fmt fmt-check tidy docker clean

all: fmt-check vet test build ## 完整检查: 格式 + vet + 测试 + 构建

build: ## 构建网关二进制
	$(GO) build -o bin/$(BINARY) $(PKG)

run: build ## 构建并运行 (默认读取 ./config/config.yaml)
	./bin/$(BINARY)

test: ## 运行单元测试
	$(GO) test ./...

race: ## 以竞态检测模式运行测试
	$(GO) test -race ./...

vet: ## 静态检查
	$(GO) vet ./...

fmt: ## 格式化代码
	$(GO) fmt ./...

fmt-check: ## 检查格式 (不通过则失败)
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

tidy: ## 整理依赖
	$(GO) mod tidy

docker: ## 构建 Docker 镜像
	docker build -t $(IMAGE) .

clean: ## 清理构建产物
	rm -rf bin/
