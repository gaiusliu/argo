// argod 进程入口，装配所有域模块并启动 HTTP 服务。
package main

import (
	"log/slog"
	"net/http"
	"os"

	"argo/src2/pact"
	"argo/src2/server"
)

func main() {
	// TODO：替换为 logx.NewLogWriter 初始化
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cs := &stubConfigSource{} // TODO：替换为 fileConfigSource
	client := &http.Client{}
	srv := server.New(cs, client)

	slog.Info("argod starting", "addr", ":8080")
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

// stubConfigSource 桩配置源。
// TODO：实现从配置文件加载 pact.AgentCfg
type stubConfigSource struct{}

func (s *stubConfigSource) Load() (pact.AgentCfg, error) {
	return pact.AgentCfg{}, nil
}
