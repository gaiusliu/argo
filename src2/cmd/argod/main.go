// argod 进程入口，装配所有域模块并启动 HTTP 服务。
package main

import (
	"log/slog"
	"net/http"
	"os"

	"argo/src2/logx"
	"argo/src2/pact"
	"argo/src2/server"
)

func main() {
	fc, err := newFileConfig()
	if err != nil {
		slog.Error("config init failed", "error", err)
		os.Exit(1)
	}

	// 进程级：Server 配置（启动时加载一次）
	scfg, err := (&serverConfigSource{fc}).Load()
	if err != nil {
		slog.Error("load server config", "error", err)
		os.Exit(1)
	}
	logWriter := initLog(scfg)
	defer logWriter.Close()

	// 请求级：Agent 配置（每次 handlePrompt 热重载）
	agentCfg := &agentConfigSource{fc}

	client := &http.Client{}
	srv := server.New(scfg, agentCfg, client)

	slog.Info("argod starting", "addr", scfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func initLog(scfg pact.ServerCfg) *logx.LogWriter {
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
