api-gateway/  
├── cmd/  
    │   └── gateway/            // 可执行文件目录  
│       └── main.go            //  API 网关服务入口  
├── config/              // 配置管理包  
│   ├── config.go        //  配置加载和结构体  
│   └── config.yaml      //  默认配置文件  
├── handler/             // 请求处理 Handler 包  
│   ├── http_handler.go  //  HTTP 请求 Handler  
│   └── healthz_handler.go // 健康检查 Handler  
├── middleware/          // 中间件包  
│   ├── auth/            //  身份验证 Middleware  
│   │   └── apikeyauth.go // API Key 认证  
│   ├── cors/            //  CORS Middleware  
│   │   └── cors.go  
│   ├── ratelimit/       //  速率限制 Middleware  
│   │   └── ratelimit.go  
│   └── logging/         //  日志记录 Middleware  
│       └── logging.go  
├── plugin/              // 插件包 (示例)  
│   ├── plugin.go        //  插件接口定义  
│   └── example/         //  示例插件  
│       └── example_plugin.go  
├── proxy/               // 反向代理包  
│   └── proxy.go         //  反向代理核心实现  
├── log/                 // 日志包  
│   └── logger.go        //  日志初始化和封装  
├── metrics/             // 指标监控包  
│   └── metrics.go       //  指标收集和 Prometheus 暴露  
├── route/               // 路由管理包  
│   └── router.go        //  路由表管理和路由匹配  
├── pkg/                 //  通用工具包  
│   └── errors/          // 自定义错误处理  
│       └── errors.go  
└── go.mod  
└── go.sum  