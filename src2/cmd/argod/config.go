package main

import (
	"argo/src2/pact"
	"argo/src2/server"
)

// ── 进程级：Server 配置 ──

type serverConfigSource struct{ cl *server.ConfigLoader }

func (s *serverConfigSource) Load() (pact.ServerCfg, error) {
	var scfg pact.ServerCfg
	if err := s.cl.Load("server.json", &scfg); err != nil {
		return pact.ServerCfg{}, err
	}
	return scfg, nil
}
