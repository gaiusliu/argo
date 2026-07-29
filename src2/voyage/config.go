package voyage

import (
	"argo/src2/board"
	"argo/src2/deck"
	"argo/src2/press"
	"argo/src2/sight"
)

// AgentCfg 请求级配置，聚合各子模块配置。
type AgentCfg struct {
	Sight sight.SightCfg `json:"sight"`
	Press press.PressCfg `json:"press"`
	Clip  deck.ClipCfg   `json:"clip"`
	Board board.BoardCfg `json:"board"`
}
