package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apexy/config"
	"apexy/handler"
	"apexy/log"
	"apexy/metrics"
	"apexy/middleware"
	"apexy/middleware/auth"
	"apexy/middleware/cors"
	"apexy/middleware/logging"
	"apexy/middleware/ratelimit"
	"apexy/middleware/requestid"
	"apexy/plugin"
	"apexy/plugin/example"
	"apexy/proxy"
	"apexy/route"
)

// defaultConfigPath 默认配置文件路径。
// 可通过命令行参数 -config 或环境变量 APEXY_CONFIG 覆盖 (参数优先级更高)。
const defaultConfigPath = "./config/config.yaml"

// resolveConfigPath 解析配置文件路径, 优先级: -config 参数 > APEXY_CONFIG 环境变量 > 默认值
func resolveConfigPath() string {
	configPath := flag.String("config", "", "path to the configuration file (yaml)")
	flag.Parse()

	if *configPath != "" {
		return *configPath
	}
	if envPath := os.Getenv("APEXY_CONFIG"); envPath != "" {
		return envPath
	}
	return defaultConfigPath
}

func main() {
	// 1. 加载配置 (含合法性校验, 配置错误时 fail-fast)
	configPath := resolveConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// log.Fatalf 内部调用 zap 的 Fatal, 会直接以非零状态码退出进程
		log.Fatalf("Failed to load config from %s: %v", configPath, err)
	}

	// 2. 初始化 Logger 日志服务
	logger := log.NewLogger(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output, cfg.Log.File)
	log.SetGlobalLogger(logger)
	log.Infof("Starting API Gateway...")

	// 3. 初始化 Metrics
	metricsServer := metrics.NewMetricsServer(cfg.Metrics.Port)
	metricsServer.Start()
	log.Infof("Metrics server started on port %d", cfg.Metrics.Port)

	// 4. 初始化 Router 路由服务
	router := route.NewRouter()
	if err := router.LoadRoutesFromConfig(cfg.Routes); err != nil {
		log.Fatalf("Failed to load routes: %v", err)
	}
	log.Infof("Loaded %d routes", len(cfg.Routes))

	// 5. 初始化 Reverse Proxy
	reverseProxy := proxy.NewReverseProxy()

	// 6. 注册插件。
	// 插件与中间件形态一致 (http.Handler 包装器), 在这里显式注册后统一接入中间件链。
	// 实际使用时可以按需注册自己的插件, 或改造为根据配置文件动态加载。
	plugin.MustRegister(example.NewExamplePlugin(map[string]interface{}{"demo": true}))

	// 7. 初始化 Middleware 链
	middlewareChain := middleware.NewChainBuilder()
	middlewareChain.AddMiddleware(logging.LoggingMiddleware(logger, metricsServer)) // 日志记录 + 指标上报
	middlewareChain.AddMiddleware(requestid.RequestIDMiddleware())                  // 请求 ID (链路追踪)
	middlewareChain.AddMiddleware(cors.CORSMiddleware(cfg.CORS))                    // CORS
	middlewareChain.AddMiddleware(auth.APIKeyAuthMiddleware(cfg.Auth.APIKeyAuth))   // API Key 认证
	middlewareChain.AddMiddleware(ratelimit.RateLimitMiddleware(cfg.RateLimit))     // 速率限制
	// 接入已注册的插件 (按注册顺序追加在内置中间件之后)
	for _, p := range plugin.Plugins() {
		middlewareChain.AddMiddleware(p.Handle)
		log.Infof("Plugin registered: %s", p.Name())
	}

	// 8. 初始化 HTTP Handler
	httpHandler := handler.NewHTTPHandler(router, reverseProxy, middlewareChain)

	// 9. 初始化 健康检查 Handler
	healthzHandler := handler.NewHealthzHandler()

	// 10. 注册 Handler
	// 注意: /metrics 不再注册到主服务端口, 指标统一由独立的 Metrics 端口 (cfg.Metrics.Port) 暴露,
	// 避免同一指标端点在两个端口重复暴露, 也避免业务流量与监控流量混在一起。
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpHandler.ServeHTTP)
	mux.HandleFunc("/healthz", healthzHandler.ServeHTTP)

	// 11. 启动 HTTP Server
	// 超时均可通过配置调整; WriteTimeout 默认 0 (不限制), 由路由级 timeout_seconds
	// 控制后端等待时间, 避免 Server 写超时先于路由超时到期导致 504 响应无法送达客户端。
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout(),
		WriteTimeout: cfg.Server.WriteTimeout(),
		IdleTimeout:  cfg.Server.IdleTimeout(),
	}

	go func() {
		// 根据配置决定以 HTTPS 还是 HTTP 方式监听
		if cfg.Server.TLS.Enabled {
			log.Infof("HTTPS server listening on port %d (tls enabled)", cfg.Server.Port)
			if err := server.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS server ListenAndServeTLS failed: %v", err)
			}
			return
		}
		log.Infof("HTTP server listening on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server ListenAndServe failed: %v", err)
		}
	}()

	// 12. 优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Infof("Shutting down API Gateway...")

	// 创建带超时的关闭上下文: 最多等待 5 秒让存量请求处理完毕,
	// 超时后强制关闭, 避免个别长请求阻塞进程退出。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 先关闭主 HTTP 服务 (停止接收新请求并等待存量请求完成)
	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("API Gateway forced to shutdown: %v", err)
	}
	// 再关闭 Metrics 服务
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Errorf("Metrics server forced to shutdown: %v", err)
	}

	log.Infof("API Gateway exiting")
}
