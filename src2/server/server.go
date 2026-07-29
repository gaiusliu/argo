// Package server 请求级装配层，创建各域模块实例并注入 Voyage。
package server

import (
	"context"
	"net/http"

	"argo/src2/pact"
	"argo/src2/sight"
)

// Serve 处理单次 prompt 请求。
// TODO：实现完整的装配流程——ConfigSource.Load() → 构造 sight/deck/lore/board → voyage.SetSail() → SSE 推送
func Serve(ctx context.Context, cs pact.ConfigSource, client *http.Client, model string) error {
	cfg, err := cs.Load()
	if err != nil {
		return err
	}
	s, err := sight.New(cfg.Sight, model, nil, client)
	if err != nil {
		return err
	}
	_ = s
	// TODO：构造 deck、lore、board、journal，注入 voyage.SetSail()
	return nil
}
