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

var logWriter *logx.LogWriter

func main() {
	cs := &stubConfigSource{} // TODO：替换为 fileConfigSource

	// 从 ConfigSource 加载进程级配置 → 初始化日志
	scfg, err := cs.LoadServer()
	if err != nil {
		slog.Error("load server config", "error", err)
		os.Exit(1)
	}
	initLog(scfg)
	defer func() { _ = logWriter.Close() }()

	client := &http.Client{}
	srv := server.New(cs, client)

	slog.Info("argod starting", "addr", scfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func initLog(scfg pact.ServerCfg) {
	w, err := logx.New(scfg.Log)
	if err != nil {
		slog.Error("log init failed", "error", err)
		os.Exit(1)
	}
	logWriter = w
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
}

// stubConfigSource 桩配置源。
// TODO：实现从配置文件加载 pact.AgentCfg + pact.ServerCfg
type stubConfigSource struct{}

func (s *stubConfigSource) Load() (pact.AgentCfg, error) {
	return pact.AgentCfg{}, nil
}

func (s *stubConfigSource) LoadServer() (pact.ServerCfg, error) {
	return pact.ServerCfg{Addr: ":8080"}, nil
}
