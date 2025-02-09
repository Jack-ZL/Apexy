package log

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 接口，定义日志操作
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})
}

// zapLogger 结构体，封装 zap logger
type zapLogger struct {
	logger *zap.Logger
}

var globalLogger Logger // 全局 Logger 实例
var once sync.Once      // 用于单例模式

// NewLogger 创建 Logger 实例
func NewLogger(levelStr string, format string, output string, file string) Logger {
	once.Do(func() { // 单例模式，只初始化一次
		level := zap.InfoLevel // 默认 Info 级别
		switch strings.ToLower(levelStr) {
		case "debug":
			level = zap.DebugLevel
		case "info":
			level = zap.InfoLevel
		case "warn":
			level = zap.WarnLevel
		case "error":
			level = zap.ErrorLevel
		case "fatal":
			level = zap.FatalLevel
		case "panic":
			level = zap.PanicLevel
		}

		var encoder zapcore.Encoder
		if strings.ToLower(format) == "json" {
			encoder = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		} else {
			encoder = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()) // text format
		}

		var writeSyncer zapcore.WriteSyncer
		if strings.ToLower(output) == "file" && file != "" {
			// 使用 lumberjack 进行日志切割
			lumberjackLogger := &lumberjack.Logger{
				Filename:   file,
				MaxSize:    100,   // 每个日志文件最大 100MB
				MaxBackups: 5,     // 最多保留 5 个备份
				MaxAge:     7,     // 最多保留 7 天
				Compress:   false, // 不压缩
			}
			writeSyncer = zapcore.AddSync(lumberjackLogger)
		} else {
			writeSyncer = zapcore.AddSync(os.Stdout) // 默认输出到 stdout
		}

		core := zapcore.NewCore(encoder, writeSyncer, level)
		globalLogger = &zapLogger{logger: zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))} // 添加 caller 和 stacktrace
	})
	return globalLogger // 返回全局 Logger 实例
}

// SetGlobalLogger 设置全局 Logger 实例
func SetGlobalLogger(logger Logger) {
	globalLogger = logger
}

// 获取全局 Logger 实例
func GetGlobalLogger() Logger {
	if globalLogger == nil {
		// 如果全局 Logger 尚未初始化，则创建一个默认的 Logger (输出到 stdout, Info 级别)
		globalLogger = NewLogger("info", "text", "stdout", "")
	}
	return globalLogger
}

// Debugf 格式化 Debug 日志
func Debugf(format string, args ...interface{}) {
	GetGlobalLogger().Debugf(format, args...)
}

// Infof 格式化 Info 日志
func Infof(format string, args ...interface{}) {
	GetGlobalLogger().Infof(format, args...)
}

// Warnf 格式化 Warn 日志
func Warnf(format string, args ...interface{}) {
	GetGlobalLogger().Warnf(format, args...)
}

// Errorf 格式化 Error 日志
func Errorf(format string, args ...interface{}) {
	GetGlobalLogger().Errorf(format, args...)
}

// Fatalf 格式化 Fatal 日志
func Fatalf(format string, args ...interface{}) {
	GetGlobalLogger().Fatalf(format, args...)
}

// Panicf 格式化 Panic 日志
func Panicf(format string, args ...interface{}) {
	GetGlobalLogger().Panicf(format, args...)
}

// Debug Debug 日志
func Debug(args ...interface{}) {
	GetGlobalLogger().Debug(args...)
}

// Info Info 日志
func Info(args ...interface{}) {
	GetGlobalLogger().Info(args...)
}

// Warn Warn 日志
func Warn(args ...interface{}) {
	GetGlobalLogger().Warn(args...)
}

// Error Error 日志
func Error(args ...interface{}) {
	GetGlobalLogger().Error(args...)
}

// Fatal Fatal 日志
func Fatal(args ...interface{}) {
	GetGlobalLogger().Fatal(args...)
}

// Panic Panic 日志
func Panic(args ...interface{}) {
	GetGlobalLogger().Panic(args...)
}

// 实现 Logger 接口的方法 (zapLogger 结构体)

func (l *zapLogger) Debugf(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

func (l *zapLogger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

func (l *zapLogger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

func (l *zapLogger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

func (l *zapLogger) Fatalf(format string, args ...interface{}) {
	l.logger.Fatal(fmt.Sprintf(format, args...))
}

func (l *zapLogger) Panicf(format string, args ...interface{}) {
	l.logger.Panic(fmt.Sprintf(format, args...))
}

func (l *zapLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

func (l *zapLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

func (l *zapLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

func (l *zapLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

func (l *zapLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(fmt.Sprint(args...))
}

func (l *zapLogger) Panic(args ...interface{}) {
	l.logger.Panic(fmt.Sprint(args...))
}
