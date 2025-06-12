package main

import (
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
	"apexy/proxy"
	"apexy/route"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig("./config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		os.Exit(1)
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
		os.Exit(1)
	}
	log.Infof("Loaded %d routes", len(cfg.Routes))

	// 5. 初始化 Reverse Proxy
	reverseProxy := proxy.NewReverseProxy()

	// 6. 初始化 Middleware 链
	middlewareChain := middleware.NewChainBuilder()
	middlewareChain.AddMiddleware(logging.LoggingMiddleware(logger))              // 日志记录
	middlewareChain.AddMiddleware(cors.CORSMiddleware(cfg.CORS))                  // CORS
	middlewareChain.AddMiddleware(auth.APIKeyAuthMiddleware(cfg.Auth.APIKeyAuth)) // API Key 认证
	middlewareChain.AddMiddleware(ratelimit.RateLimitMiddleware(cfg.RateLimit))   // 速率限制
	// 可以添加更多 Middleware

	// 7. 初始化 HTTP Handler
	httpHandler := handler.NewHTTPHandler(router, reverseProxy, middlewareChain)

	// 8. 初始化 健康检查 Handler
	healthzHandler := handler.NewHealthzHandler()

	// 9. 注册 Handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpHandler.ServeHTTP)
	mux.HandleFunc("/healthz", healthzHandler.ServeHTTP)
	mux.HandleFunc("/metrics", metricsServer.ServeHTTP) // 注册 metrics 端点

	// 10. 启动 HTTP Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Infof("HTTP server listening on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server ListenAndServe failed: %v", err)
			os.Exit(1)
		}
	}()

	// 11. 优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Infof("Shutting down API Gateway...")

	// 创建关闭的上下文
	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// defer cancel()
	// if err := server.Shutdown(ctx); err != nil {
	// 	log.Errorf("API Gateway forced to shutdown: %v", err)
	// }

	log.Infof("API Gateway exiting")
}
