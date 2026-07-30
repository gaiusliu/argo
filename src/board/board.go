// Package board 审批引擎——基于规则匹配和用户审批记录决定工具调用放行/拒绝/询问。
package board

import (
	"sync"

	"argo/src/pact"
)

// 审核动作：ActionApprove 放行 / ActionAsk 确认 / ActionDeny 拒绝。
const (
	ActionApprove = iota + 1
	ActionAsk
	ActionDeny
)

// 规则类型：RuleTypeIron 铁律（优先）/ RuleTypeCord 绳索（后备）。
const (
	RuleTypeIron = iota + 1
	RuleTypeCord
)

// Rule 单条审批规则，定义工具名+参数模式的匹配条件和匹配后的动作。
type Rule struct {
	// Hand 工具名，对应 deck.HandMeta.Name
	Hand string `json:"hand"`
	// Pattern 参数匹配模式
	Pattern string `json:"pattern"`
	// Action 匹配后执行的动作，取值见 ActionApprove / ActionAsk / ActionDeny
	Action int `json:"action"`
	// Type 规则类型，取值见 RuleTypeIron / RuleTypeCord
	Type int `json:"type"`
}

// BoardCfg 审批规则配置。
type BoardCfg struct {
	// Iron 铁律规则，优先匹配，直接生效
	Iron []Rule `json:"iron"`
	// Cord 绳索规则，铁律未匹配时生效
	Cord []Rule `json:"cord"`
}

// AyeEntry 船长许可条目，供持久化快照使用。
type AyeEntry struct {
	// Hand 工具名
	Hand string
	// Pattern 参数匹配模式
	Pattern string
}

// Board 审批引擎，存储铁律/绳索规则、船长许可记录。
type Board struct {
	iron, cord []Rule
	aye        map[string]struct{}
	mu         sync.RWMutex
}

// New 创建审批引擎。iron/cord 分别为铁律/绳索规则。
func New(iron, cord []Rule) *Board {
	return &Board{
		iron: iron,
		cord: cord,
		aye:  make(map[string]struct{}),
	}
}

// Read 审批 Omen，返回审核动作和理由。
// 匹配优先级：铁律规则 → 绳索规则 → 船长许可记录 → 询问。
func (b *Board) Read(omen pact.Omen) (int, string) {
	if v, r := match(b.iron, omen); v != 0 {
		return v, r
	}
	if v, r := match(b.cord, omen); v != 0 {
		return v, r
	}
	if b.hasAye(omen) {
		return ActionApprove, "user approved"
	}
	return ActionAsk, "permission required"
}

// Pass 记录船长对某 Omen 的许可。
func (b *Board) Pass(omen pact.Omen) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.aye[scopeKey(omen)] = struct{}{}
}
