# 多阶段构建: 编译阶段
FROM golang:1.25-alpine AS builder

WORKDIR /src

# 先复制依赖清单, 利用 Docker 层缓存加速重复构建
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO 禁用产出静态二进制, 便于在 scratch/alpine 中运行
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/apexy ./cmd/gateway

# 运行阶段: 最小镜像
FROM alpine:3.20

# ca-certificates: HTTPS 后端需要; tzdata: 日志时间戳时区
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/apexy /app/apexy
# 只带示例配置进镜像, 实际部署时通过挂载或 APEXY_CONFIG 指定真实配置
COPY config/config.example.yaml /app/config/config.example.yaml

EXPOSE 8080 9090

ENV APEXY_CONFIG=/app/config/config.example.yaml

ENTRYPOINT ["/app/apexy"]
