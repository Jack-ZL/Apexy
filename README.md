# Apexy

一个用 Go 编写的高性能 API 网关，支持反向代理、API Key 认证、令牌桶限流、CORS、请求追踪（X-Request-ID）、请求/响应改写、路由级超时、Prometheus 监控指标、插件扩展和 HTTPS，适用于学习网关原理或作为轻量级生产网关使用。

---

## 目录

- [核心特性](#核心特性)
- [部署形态：它在架构中的位置](#部署形态它在架构中的位置)
- [工作原理](#工作原理)
- [快速开始](#快速开始)
- [配置详解](#配置详解)
- [功能使用示例](#功能使用示例)
- [开启 HTTPS (TLS)](#开启-https-tls)
- [插件开发教程](#插件开发教程)
- [Docker 部署](#docker-部署)
- [Prometheus 监控接入](#prometheus-监控接入)
- [常见问题 FAQ](#常见问题-faq)
- [开发指南](#开发指南)
- [目录结构](#目录结构)

---

## 核心特性

| 特性 | 说明 |
|---|---|
| **反向代理** | 将请求转发到后端服务，支持 Header/Query 参数改写、路由级超时（超时返回 504） |
| **路由匹配** | 精确匹配 + 路径段边界前缀匹配（`/api/v1/users` 可匹配 `/api/v1/users/123`），Method 不区分大小写，留空匹配所有方法 |
| **API Key 认证** | 全局或按路由开关，常量时间比较（防时序攻击），日志中 Key 自动脱敏 |
| **速率限制** | 令牌桶算法，按客户端 IP 限流，支持全局/路由两级策略，空闲客户端自动回收内存 |
| **CORS** | Origin 白名单、预检请求校验、`Vary: Origin`；凭证模式下自动拒绝通配符（安全加固） |
| **请求追踪** | 自动生成/透传 `X-Request-ID`，便于全链路日志排查 |
| **监控指标** | Prometheus 请求计数、延迟直方图，独立端口暴露，不混入业务流量 |
| **插件系统** | 实现 `plugin.Plugin` 接口即可扩展自定义中间件逻辑 |
| **HTTPS** | 配置证书即可开启 TLS 监听 |
| **优雅关闭** | SIGINT/SIGTERM 信号触发，排空存量连接后退出 |
| **严格配置** | YAML 未知字段启动即报错，配置校验失败会给出明确原因，避免"配错了但没发现" |

## 部署形态：它在架构中的位置

Apexy 是一个**独立部署的应用**（类似 Nginx / Kong / Traefik），而**不是** `go get` 引入项目的依赖库（beego / gin 那种形态）。你的项目与它零代码耦合，只通过 HTTP 流量连接：

```
客户端请求 ──→ Apexy (网关, :8080) ──→ 你的项目 (后端服务, :8081/:8082/...)
 (浏览器/App)         │
                      └─ 在这一层统一完成:
                         认证 (401 拦截无效 Key)
                         限流 (429 拦截超频请求)
                         CORS / 日志 / Request-ID / 监控指标
                         请求改写 → 转发 → 响应改写
```

要点：

- **客户端只认识网关**：只访问 Apexy 的地址，不知道也不需要知道后端服务在哪
- **你的服务藏在网关后面**：可以只监听内网，不对外暴露；后端用什么语言写都行（Java/Python/Node/Go...），无需引入任何 Go 依赖
- **一个网关代理 N 个后端**：按 `config.yaml` 的路由规则分流，如 `/api/v1/users` → 用户服务、`/api/v2/products` → 订单服务
- **非法请求在网关层拦截**：无 Key (401)、超限 (429)、无路由 (404) 的请求根本不会到达你的服务

使用方式是"克隆 → 构建 → 部署"，而非添加依赖：

```bash
git clone https://github.com/Jack-ZL/Apexy.git
cd Apexy && make build          # 编译出独立可执行文件 bin/apexy
./bin/apexy -config config/config.yaml   # 作为独立进程运行在你的服务前面
```

生产环境的典型完整链路：

```
客户端 → DNS/CDN → 负载均衡 (Nginx/云 LB) → Apexy → 你的服务集群
```

此时 Apexy 位于可信反向代理之后，应开启 `rate_limit.trust_forwarded_headers: true`，
从 `X-Forwarded-For` 获取真实客户端 IP 进行限流（直连公网部署时保持默认 `false`，防止伪造头绕过限流）。

## 工作原理

一个请求进入网关后的完整处理流程：

```
客户端请求
   │
   ▼
路由匹配 (route.Router)          ── 未匹配 → 404
   │  匹配到的路由注入请求 Context
   ▼
中间件链 (按顺序执行):
   1. logging     记录请求日志 + 上报 Prometheus 指标
   2. requestid   生成/透传 X-Request-ID
   3. cors        跨域处理 (预检请求直接返回 204/403)
   4. auth        API Key 校验                ── 失败 → 401
   5. ratelimit   令牌桶限流                  ── 超限 → 429
   6. plugins     已注册的插件 (按注册顺序)
   ▼
反向代理 (proxy.ReverseProxy)
   │  应用请求改写 → 转发后端 (受 timeout_seconds 约束)
   │  应用响应改写 → 返回客户端
   │                                          ── 后端不可达 → 502
   │                                          ── 后端超时   → 504
   ▼
客户端响应
```

**网关状态码约定**：

| 状态码 | 含义 |
|---|---|
| 404 | 没有匹配到任何路由 |
| 401 | API Key 缺失或无效 |
| 429 | 触发速率限制 |
| 502 | 后端服务不可达/连接失败 |
| 504 | 后端响应超过路由的 `timeout_seconds` |

## 快速开始

### 1. 环境要求

- Go 1.24+（推荐 1.25+）
- make（可选，也可以直接用 go 命令）

### 2. 构建

```bash
git clone https://github.com/Jack-ZL/Apexy.git
cd Apexy

make build          # 产物: bin/apexy
# 等价于: go build -o bin/apexy ./cmd/gateway
```

### 3. 准备配置

```bash
cp config/config.example.yaml config/config.yaml
```

配置文件按以下优先级解析（高 → 低）：

1. 命令行参数 `-config /path/to/config.yaml`
2. 环境变量 `APEXY_CONFIG=/path/to/config.yaml`
3. 默认路径 `./config/config.yaml`

### 4. 启动一个测试后端

网关只是转发层，需要一个后端服务才能看到完整效果。用 Python 一行起一个 echo 后端（会把收到的请求信息原样返回，方便观察网关行为）：

```bash
cat > /tmp/backend.py <<'EOF'
import json, sys, time
from http.server import BaseHTTPRequestHandler, HTTPServer

class H(BaseHTTPRequestHandler):
    def _handle(self):
        # 支持 X-Delay 头模拟慢后端, 用于演示路由超时 (504)
        delay = float(self.headers.get("X-Delay", "0"))
        if delay:
            time.sleep(delay)
        body = json.dumps({
            "backend_port": self.server.server_port,
            "method": self.command,
            "path": self.path,
            "x_request_id": self.headers.get("X-Request-ID"),
        }, indent=2).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    do_GET = do_POST = do_PUT = do_DELETE = _handle
    def log_message(self, *a): pass

HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
EOF

python3 /tmp/backend.py 8081 &   # 示例配置中 /api/v1/users 的后端
python3 /tmp/backend.py 8083 &   # 示例配置中 /public/data 的后端
```

### 5. 启动网关并验证

```bash
./bin/apexy                                # 读取 ./config/config.yaml
# 或: ./bin/apexy -config /path/to.yaml
# 或: make run
```

看到以下日志即启动成功：

```
INFO  HTTP server listening on port 8080
INFO  Prometheus metrics endpoint listening on :9090/metrics
INFO  Plugin registered: ExamplePlugin
```

验证：

```bash
curl http://localhost:8080/healthz                       # 健康检查 → 200
curl http://localhost:8080/public/data                   # 公共路由 (免认证) → 200
curl -H "X-API-Key: your-api-key-1" \
     http://localhost:8080/api/v1/users/123              # 认证路由 → 200
```

## 配置详解

完整带注释的模板见 [config/config.example.yaml](config/config.example.yaml)。
**注意：配置采用严格模式，出现未知字段（如拼写错误）会直接启动失败并指出行号。**

### server — 服务监听

```yaml
server:
  port: 8080                 # 主服务端口 (必填, 1-65535)
  read_timeout_seconds: 15   # 读取请求超时, 0 使用默认 15s
  write_timeout_seconds: 0   # 写响应超时, 0 = 不限制 (推荐)
  idle_timeout_seconds: 120  # keep-alive 空闲超时, 0 使用默认 120s
  tls:                       # HTTPS 配置 (可选)
    enabled: false
    cert_file: ""            # PEM 证书路径
    key_file: ""             # PEM 私钥路径
```

> ⚠️ `write_timeout_seconds` 若设置非 0 值，**必须大于所有路由的 `timeout_seconds`**，
> 否则后端超时时网关来不及写回 504，客户端会收到空响应。默认 0（不限制）即可，
> 由路由级超时控制后端等待时间——这也是网关支持 SSE/长轮询的常见做法。

### log — 日志

```yaml
log:
  level: debug        # debug / info / warn / error / fatal / panic
  format: text        # text (开发) 或 json (生产, 便于日志系统采集)
  output: stdout      # stdout 或 file
  file: "api-gateway.log"   # output=file 时的日志路径
```

### metrics — 监控指标

```yaml
metrics:
  port: 9090          # 独立指标端口 (必须与 server.port 不同)
```

指标只在该端口的 `/metrics` 暴露，主端口访问 `/metrics` 返回 404。

### auth — 全局 API Key 认证

```yaml
auth:
  api_key_auth:
    enabled: true             # 全局开关
    header_name: X-API-Key    # 携带 Key 的请求头名称
    keys:                     # 合法 Key 列表
      - "your-api-key-1"
      - "your-api-key-2"
```

> 生产环境建议：使用足够随机的长 Key，并通过配置管理/密钥系统下发，不要提交到仓库。

### rate_limit — 全局限流

```yaml
rate_limit:
  enabled: true
  trust_forwarded_headers: false  # 是否信任 X-Real-IP/X-Forwarded-For
  general:
    requests_per_minute: 1000     # 全局默认: 每客户端 IP 每分钟 1000 次
  routes:                         # 按路由路径覆盖全局策略 (可选)
    /api/v1/users:
      requests_per_minute: 500    # 注意: 这里直接写策略, 没有 policy 包装层
```

- 限流按**客户端 IP** 独立计数（令牌桶算法，支持突发流量平滑恢复）。
- `trust_forwarded_headers` 默认 `false`：直接取 TCP 连接地址作为客户端 IP，防止伪造
  `X-Forwarded-For` 绕过限流。**仅当网关部署在可信反向代理（如 Nginx/云 LB）之后时才开启**。
- 策略优先级：**路由内联 `rate_limit.policy` > 全局 `rate_limit.routes[路径]` > 全局 `general`**。
- 空闲超过 10 分钟的客户端桶会被自动回收，防止 IP 数量增长导致内存泄漏。

### cors — 跨域

```yaml
cors:
  enabled: true
  max_age_seconds: 600        # 预检结果缓存时间
  allowed_origins:
    - "https://app.example.com"   # 生产环境建议列明确 Origin
  allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
  allowed_headers: ["Origin", "Content-Type", "Accept", "Authorization"]
  exposed_headers: ["Content-Length"]
  allow_credentials: true
```

安全规则：**`allow_credentials: true` 时通配符 `*` 不生效**（启动时打印 WARN），
只有显式列出的 Origin 才允许携带凭证跨域；预检请求会校验
`Access-Control-Request-Method`/`-Headers`，不合法返回 403，合法返回 204。

### routes — 路由表（核心）

```yaml
routes:
  - path: /api/v1/users            # 必填。精确或路径段边界前缀匹配
    method: GET                    # 可选。留空匹配所有方法; 不区分大小写
    backend: http://localhost:8081 # 必填。后端地址 (可带基础路径, 如 http://host:8081/v2)
    timeout_seconds: 10            # 可选。后端超时, 0/省略 = 不限制, 超时返回 504

    auth:
      api_key_auth_enabled: true   # 三态: 省略=继承全局, true/false=覆盖全局
    rate_limit:
      enabled: true                # 三态: 同上
      policy:
        requests_per_minute: 200   # 路由专属限流策略 (最高优先级)

    request_transform:             # 转发给后端前的请求改写 (可选)
      headers_to_add:
        X-Gateway: "apexy"
      headers_to_remove: ["X-Powered-By"]
      query_params_to_add:
        app_version: "1.0"
      query_params_to_remove: ["debug"]
    response_transform:            # 返回客户端前的响应改写 (可选)
      headers_to_add:
        X-Gateway-Version: "1.0"
      headers_to_remove: ["Server"]
```

**路由匹配规则**：

| 请求路径 | 路由 `/api/v1/users` 是否匹配 |
|---|---|
| `/api/v1/users` | ✅ 精确匹配 |
| `/api/v1/users/123` | ✅ 段边界前缀匹配 |
| `/api/v1/usersabc` | ❌ 不是段边界（防止 `/users` 误吞 `/usersadmin`） |
| `/api/v1` | ❌ 前缀比路由短 |

转发时**保留子路径**：`/api/v1/users/123` → `http://localhost:8081/api/v1/users/123`；
若 backend 带基础路径 `http://localhost:8081/v2`，则转发到 `/v2/api/v1/users/123`。

多条路由都能匹配时，**按配置顺序取第一条**——因此更具体的路由（长路径）应写在更通用的路由（短路径）之前；同一路径重复定义会在启动校验时报错。

## 功能使用示例

以下示例基于 `config/config.example.yaml` 的默认配置（Key 为 `your-api-key-1`）。

### 健康检查

```bash
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
```

### 认证（401 / 200）

```bash
# 不带 Key → 401
curl -i http://localhost:8080/api/v1/users
# HTTP/1.1 401 Unauthorized

# 带错误 Key → 401 (日志中 Key 已脱敏, 只显示前 4 位)
curl -i -H "X-API-Key: wrong-key" http://localhost:8080/api/v1/users

# 带正确 Key → 200
curl -i -H "X-API-Key: your-api-key-1" http://localhost:8080/api/v1/users
```

路由级开关：示例配置中 `/public/data` 设置了 `api_key_auth_enabled: false`，无需 Key 直接访问。

### 限流（429）

`/api/v1/users` 限 200 次/分钟，快速发 210 个请求观察：

```bash
for i in $(seq 1 210); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -H "X-API-Key: your-api-key-1" http://localhost:8080/api/v1/users
done | sort | uniq -c
# 大部分 200, 尾部出现 429 (令牌桶持续回填, 具体比例取决于发压速度)
# 例如: 206 个 200, 4 个 429
```

### CORS 预检

默认示例配置是 `allowed_origins: ["*"]` + `allow_credentials: true`——凭证模式下通配符会被忽略（启动日志有 WARN），此时任何预检都返回 403。先把前端 Origin 显式加入白名单再重启网关：

```yaml
cors:
  allowed_origins:
    - "https://app.example.com"
```

```bash
# 合法预检 (Origin 在白名单中) → 204
curl -i -X OPTIONS http://localhost:8080/public/data \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: GET"
# 响应包含: Access-Control-Allow-Origin / Allow-Methods / Max-Age / Vary: Origin

# 非法方法 (不在 allowed_methods 中) → 403
curl -i -X OPTIONS http://localhost:8080/public/data \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: PATCH"

# 未知 Origin (不在白名单中) → 403
curl -i -X OPTIONS http://localhost:8080/public/data \
  -H "Origin: https://evil.com" \
  -H "Access-Control-Request-Method: GET"
```

### 请求追踪（X-Request-ID）

```bash
# 网关自动生成
curl -i http://localhost:8080/public/data | grep -i x-request-id
# X-Request-Id: 148bb250d4a74133b6670168188592e9

# 客户端自带则透传 (用于串联上游链路)
curl -s -H "X-Request-ID: my-trace-001" http://localhost:8080/public/data
# 后端 echo 出的 x_request_id == "my-trace-001"
```

网关日志中每条请求记录都带该 ID，可用它 grep 全链路日志。

### 请求/响应改写

```bash
curl -i -H "X-API-Key: your-api-key-1" \
  "http://localhost:8080/api/v1/users?debug=1&page=2"
# 请求侧: 后端收到的 query 中 debug 被移除, 新增 app_version=1.0
# 响应侧: 响应头新增 X-Gateway-Version: 1.0, 移除 Server
```

### 路由超时（504）

给路由配置 `timeout_seconds: 10` 后，让后端睡 11 秒（可用支持延迟的测试后端）：

```bash
curl -i -m 15 -H "X-API-Key: your-api-key-1" \
  -H "X-Delay: 11" http://localhost:8080/api/v1/users
# HTTP/1.1 504 Gateway Timeout
# gateway timeout: backend timeout after 10s
```

### 监控指标

```bash
curl -s http://localhost:9090/metrics | grep api_gateway
# api_gateway_request_total{method="GET",path="/api/v1/users",status="200"} 5
# api_gateway_request_duration_seconds_bucket{...}
```

> 指标 label 使用**路由路径**（而非原始 URL），避免 `/users/1`、`/users/2` 造成基数爆炸。

## 开启 HTTPS (TLS)

1. 准备证书（生产用 CA 签发的证书；本地测试可自签）：

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout server.key -out server.crt -subj "/CN=localhost"
```

2. 修改配置：

```yaml
server:
  port: 8443
  tls:
    enabled: true
    cert_file: ./server.crt
    key_file: ./server.key
```

3. 启动后以 HTTPS 访问：

```bash
curl -k https://localhost:8443/healthz    # -k 跳过自签证书校验
```

## 插件开发教程

插件本质是一个中间件（`http.Handler` 包装器），可以在请求前后插入任意逻辑。

### 1. 实现插件接口

```go
// plugin.Plugin 接口定义:
//   Name() string                          — 插件名称 (不可与已注册插件重复)
//   Handle(next http.Handler) http.Handler — 包装下游 Handler
```

参考 `plugin/example/example_plugin.go` 编写自己的插件：

```go
package myplugin

import (
    "net/http"
    "apexy/log"
)

type HeaderInjector struct{}

func (p *HeaderInjector) Name() string { return "HeaderInjector" }

func (p *HeaderInjector) Handle(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 请求前逻辑: 所有经过网关的请求统一打标
        r.Header.Set("X-Via", "apexy")
        next.ServeHTTP(w, r)
        // 请求后逻辑 (响应已发出, 只能做记录类操作)
        log.Debugf("HeaderInjector processed: %s", r.URL.Path)
    })
}
```

### 2. 注册插件

在 `cmd/gateway/main.go` 中注册（网关启动时会把所有已注册插件按顺序追加到内置中间件之后）：

```go
import "apexy/plugin/myplugin"

plugin.MustRegister(&myplugin.HeaderInjector{})
```

### 3. 验证

重启网关，日志出现 `Plugin registered: HeaderInjector` 即生效。

注意事项：

- 同名插件重复注册会报错（`Register` 返回 error，`MustRegister` 直接 panic），防止插件被执行两次。
- 插件执行顺序 = 注册顺序，位于 auth/ratelimit **之后**，因此插件里拿到的请求已通过认证。
- 插件内请使用 `apexy/log` 打日志，保持格式/级别/输出一致。

## Docker 部署

```bash
make docker                       # 构建镜像 apexy:latest
# 等价于: docker build -t apexy:latest .
```

运行（挂载真实配置，通过 `APEXY_CONFIG` 指定路径）：

```bash
docker run -d --name apexy \
  -p 8080:8080 -p 9090:9090 \
  -v $(pwd)/config/config.yaml:/app/config/config.yaml:ro \
  -e APEXY_CONFIG=/app/config/config.yaml \
  apexy:latest
```

说明：

- 镜像为多阶段构建，运行层基于 `alpine:3.20`，包含 `ca-certificates`（HTTPS 后端需要）和 `tzdata`（日志时区）。
- 镜像内默认 `APEXY_CONFIG=/app/config/config.example.yaml`（示例配置），生产务必挂载自己的配置覆盖。
- 容器内后端地址不能写 `localhost`（指向容器自身），应使用宿主机地址、容器名或 `host.docker.internal`。

## Prometheus 监控接入

在 Prometheus 的 `scrape_configs` 中添加：

```yaml
scrape_configs:
  - job_name: apexy
    scrape_interval: 15s
    static_configs:
      - targets: ["localhost:9090"]   # metrics.port 配置的端口
```

可用指标：

| 指标 | 类型 | Labels | 说明 |
|---|---|---|---|
| `api_gateway_request_total` | Counter | method, path, status | 请求总数 |
| `api_gateway_request_duration_seconds` | Histogram | method, path | 请求延迟分布 |

常用 PromQL 示例：

```promql
# QPS (按路由)
sum(rate(api_gateway_request_total[1m])) by (path)

# P99 延迟
histogram_quantile(0.99, sum(rate(api_gateway_request_duration_seconds_bucket[5m])) by (le, path))

# 5xx 错误率
sum(rate(api_gateway_request_total{status=~"5.."}[5m])) / sum(rate(api_gateway_request_total[5m]))
```

## 常见问题 FAQ

**Q1: 启动报 `field xxx not found in type config.Xxx`？**
配置采用严格模式，YAML 中存在结构体未定义的字段（通常是拼写错误或缩进错位）。按报错行号修正，或对照 `config/config.example.yaml`。

**Q2: 后端明明超时了，客户端却收到空响应而不是 504？**
`server.write_timeout_seconds` 小于等于路由 `timeout_seconds`，Server 写超时先到期关闭了连接。保持 `write_timeout_seconds: 0`（默认）或设置为大于所有路由超时的值。

**Q3: CORS 配置了 `allowed_origins: ["*"]` 但浏览器跨域仍失败？**
`allow_credentials: true` 时通配符会被忽略（安全设计，启动日志有 WARN）。把前端的具体 Origin 显式加入 `allowed_origins`，或关闭 `allow_credentials`。

**Q4: 限流被绕过了？**
检查是否有客户端伪造 `X-Forwarded-For`。默认 `trust_forwarded_headers: false` 不信任该头；仅当网关位于可信反向代理之后才开启。

**Q5: 路由 404？**
确认：① 路径是段边界前缀（`/api/v1/usersabc` 不会匹配 `/api/v1/users`）；② `method` 是否与请求方法一致（留空匹配所有）；③ 多条路由都能匹配时按配置顺序取第一条，更具体的路由要写在前面。

**Q6: 主端口访问 `/metrics` 返回 404？**
设计如此。指标只在 `metrics.port`（默认 9090）暴露，避免业务与监控流量混在一起。

**Q7: 修改配置后如何生效？**
重启网关进程（发送 SIGINT/SIGTERM 会优雅关闭，排空存量连接后再启动新进程）。

## 开发指南

```bash
make all          # 完整检查: gofmt + vet + test + build
make build        # 构建 → bin/apexy
make run          # 构建并运行
make test         # 单元测试
make race         # 竞态检测模式跑测试
make vet          # 静态检查
make fmt          # 格式化代码
make tidy         # 整理依赖
make docker       # 构建 Docker 镜像
make clean        # 清理构建产物
```

CI（GitHub Actions，`.github/workflows/ci.yml`）在每次 push/PR 时自动执行 vet + test + race。

单元测试覆盖 8 个包（route/proxy/config/plugin/auth/ratelimit/cors/requestid），提交前请确保 `make race` 通过。

## 目录结构

```
Apexy/
├── cmd/gateway/            // 可执行文件目录
│   └── main.go             // 网关入口（配置加载、中间件链组装、优雅关闭）
├── config/                 // 配置管理包
│   ├── config.go           // 配置加载（严格模式）、结构体定义、Validate 校验
│   └── config.example.yaml // 配置模板（复制为 config.yaml 使用）
├── handler/                // 请求处理 Handler 包
│   ├── http_handler.go     // HTTP Handler（路由匹配 + 中间件链 + 代理）
│   └── healthz_handler.go  // 健康检查 Handler
├── middleware/             // 中间件包
│   ├── auth/               // API Key 认证（常量时间比较、日志脱敏）
│   ├── cors/               // CORS（预检校验、Vary: Origin）
│   ├── ratelimit/          // 速率限制（令牌桶、空闲客户端回收）
│   ├── requestid/          // 请求 ID（X-Request-ID 生成/透传）
│   └── logging/            // 请求日志（支持 Flush/Hijack/Push）
├── plugin/                 // 插件包
│   ├── plugin.go           // 插件接口定义 + 全局注册表
│   └── example/            // 示例插件
├── proxy/                  // 反向代理包
│   └── proxy.go            // 代理核心（请求/响应改写、超时控制）
├── log/                    // 日志包（zap 封装）
├── metrics/                // Prometheus 指标（独立端口暴露）
├── route/                  // 路由管理（路由表、匹配、Context 注入）
├── pkg/errors/             // 自定义错误（Wrap/Unwrap/Is/As）
├── Makefile                // 构建/测试/lint/docker 任务
├── Dockerfile              // 多阶段构建镜像
├── .github/workflows/      // CI（vet + test + race）
├── go.mod
└── go.sum
```
