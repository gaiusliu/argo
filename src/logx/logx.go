// Package logx 日志扩展，提供按日期+大小自动轮转的 io.Writer。
// 命名避开标准库 log 包。
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LogCfg 日志配置。
type LogCfg struct {
	// Dir 日志目录
	Dir string `json:"dir,omitempty"`
	// Level 日志级别：debug / info / warn / error
	Level string `json:"level,omitempty"`
	// MaxSize 单文件大小上限（MB）
	MaxSize int `json:"max_size,omitempty"`
	// MaxAge 文件保留天数
	MaxAge int `json:"max_age,omitempty"`
}

// LogWriter 日志写入器，支持按日期和大小自动轮转。
type LogWriter struct {
	dir     string
	maxSize int64
	maxAge  int

	curDate string
	curSize int64
	seq     int

	file *os.File
}

// New 创建日志写入器：解析目录、创建目录、清理过期文件、打开当天日志。
func New(cfg LogCfg) (*LogWriter, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = "./logs/"
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("logx: %w", err)
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("logx: %w", err)
	}
	maxSize := int64(cfg.MaxSize)
	if maxSize <= 0 {
		maxSize = 100
	}
	maxSize *= 1024 * 1024
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = 7
	}
	w := &LogWriter{
		dir:     absDir,
		maxSize: maxSize,
		maxAge:  maxAge,
	}
	_ = w.cleanup()
	if err := w.rotate(); err != nil {
		return nil, fmt.Errorf("logx: %w", err)
	}
	return w, nil
}

// Write 实现 io.Writer。先检查轮转条件（跨天/超大小），再写入。
func (w *LogWriter) Write(p []byte) (int, error) {
	today := time.Now().Format("2006-01-02")
	if today != w.curDate || w.curSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.curSize += int64(n)
	return n, err
}

// Close 关闭当前日志文件。
func (w *LogWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// rotate 关闭旧文件，按 "logx-日期[.NNN].log" 命名打开新文件。
func (w *LogWriter) rotate() error {
	if w.file != nil {
		w.file.Close()
	}
	today := time.Now().Format("2006-01-02")
	if w.curDate == "" {
		w.curDate = today
		if latest := findLatestSeq(w.dir, today); latest >= 0 {
			w.seq = latest
		}
	} else if today == w.curDate {
		w.seq++
	} else {
		w.curDate = today
		w.seq = 0
	}
	path := w.logPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.curSize = info.Size()
	return nil
}

// logPath 按日期和序号拼出日志文件路径。
func (w *LogWriter) logPath() string {
	if w.seq == 0 {
		return filepath.Join(w.dir, "logx-"+w.curDate+".log")
	}
	return filepath.Join(w.dir, fmt.Sprintf("logx-%s.%03d.log", w.curDate, w.seq))
}

// findLatestSeq 返回当天日志文件的最大序号，无文件返回 -1。
func findLatestSeq(dir, date string) int {
	prefix := "logx-" + date
	entries, err := os.ReadDir(dir)
	if err != nil {
		return -1
	}
	maxSeq := -1
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := name[len(prefix):]
		if rest == ".log" {
			maxSeq = max(maxSeq, 0)
			continue
		}
		if len(rest) == 8 && rest[0] == '.' && strings.HasSuffix(rest, ".log") {
			if n, err := strconv.Atoi(rest[1:4]); err == nil {
				maxSeq = max(maxSeq, n)
			}
		}
	}
	return maxSeq
}

// cleanup 删除修改时间超过 maxAge 天的日志文件。
func (w *LogWriter) cleanup() error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -w.maxAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "logx-") || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(w.dir, e.Name()))
		}
	}
	return nil
}
