// Argo server 进程入口。
// 启动 HTTP 后端，承载 RunLoop、vault 写入、brig 门控、LLM 调用和工具执行。
package main

import (
	"fmt"
	"log/slog"
	"os"

	"argo/src/crew"
	"argo/src/deck"
	"argo/src/knot"
	"argo/src/server"
	"argo/src/wake"
)

func main() {
	// 加载全局配置
	cfg, err := knot.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志文件
	level := parseLevel(cfg.Log.Level)
	programLevel := new(slog.LevelVar)
	programLevel.Set(level)

	w, err := wake.NewLogWriter(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer w.Close()

	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level:     programLevel,
		AddSource: true,
	})))

	// 注册所有工具（内置 + CLI）
	if err := deck.RegisterTools(); err != nil {
		slog.Error("工具注册失败", "error", err)
		os.Exit(1)
	}

	// 获取工作目录
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("获取工作目录失败", "error", err)
		os.Exit(1)
	}

	// 初始化技能注册表
	if err := crew.Init(knot.ArgoDir(), cwd); err != nil {
		slog.Error("技能注册表初始化失败", "error", err)
		os.Exit(1)
	}

	// 启动 HTTP server
	srv := server.New(server.Config{
		Addr:    ":4090",
		WorkDir: cwd,
	})

	slog.Info("Argo server 启动", "addr", ":4090")
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server 退出", "error", err)
		os.Exit(1)
	}
}

// parseLevel 将配置字符串转为 slog.Level。
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
