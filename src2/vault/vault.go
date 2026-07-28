// Package vault 定义持久化原语——Journal（追加流）和 Checkpoint（快照）。
// 接口仅操作文本行，序列化/反序列化由上层领域包负责。
package vault

// Journal 追加流，用于 transcript 等只增不改的数据。
type Journal interface {
	// Append 批量追加记录，每条记录以换行分隔写入。
	Append(records []string) error
	// Scan 返回当前全部记录行。文件不存在时返回 nil, nil。
	Scan() ([]string, error)
}

// Checkpoint 快照，用于断点状态、审批记录等整体覆盖的数据。
type Checkpoint interface {
	// Load 读取快照内容。文件不存在时返回 nil, nil。
	Load() (string, error)
	// Save 整体覆盖写入快照。
	Save(data string) error
	// Delete 删除快照。文件不存在时不报错。
	Delete() error
}
