// argod 进程入口，装配所有域模块并启动 HTTP 服务。
package main

import (
	"log/slog"
	"os"

	"argo/src/logx"
	"argo/src/server"
)

func main() {
	cl, err := server.NewFileConfigLoader()
	if err != nil {
		slog.Error("config init failed", "error", err)
		os.Exit(1)
	}

	// 进程级：Server 配置（启动时加载一次）
	var scfg server.ServerCfg
	if err := cl.Load("server.json", &scfg); err != nil {
		slog.Error("load server config", "error", err)
		os.Exit(1)
	}
	logWriter := initLog(scfg)
	defer logWriter.Close()

	srv := server.New(scfg)

	slog.Info("argod starting", "addr", scfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func initLog(scfg server.ServerCfg) *logx.LogWriter {
	w, err := logx.New(scfg.Log)
	if err != nil {
		slog.Error("log init failed", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	return w
}
