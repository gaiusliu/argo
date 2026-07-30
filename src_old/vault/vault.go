// Package vault 是 Argo 的纯存储层，负责 session transcript 的读写。
// Vault 构造时绑定 session 目录，不知道 session ID，不知道 brig.Engine。
// Session CRUD 由上层 voyage 包负责。
package vault

import "argo/src_old/knot"

// Vault 纯存储接口。每个实例绑定一个 session 目录。
type Vault interface {
	// Append 追加 records 到 transcript.jsonl。
	Append(records []Record) error

	// Load 读取 transcript，映射为 knot.Message。
	Load() ([]knot.Message, error)
}
