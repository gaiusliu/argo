// Package logx 日志扩展，提供按日期+大小自动轮转的 io.Writer。
// 命名避开标准库 log 包。
package logx

import "argo/src2/pact"

// LogWriter 日志写入器，支持按日期和大小自动轮转。
type LogWriter struct {
	cfg pact.LogCfg
}

// New 创建日志写入器。
// TODO：实现目录创建、日志轮转、过期清理
func New(cfg pact.LogCfg) (*LogWriter, error) {
	return &LogWriter{cfg: cfg}, nil
}

// Write 实现 io.Writer。
// TODO：实现日期+大小轮转检查后再写入
func (w *LogWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// Close 关闭当前日志文件。
// TODO：实现文件关闭
func (w *LogWriter) Close() error {
	return nil
}
