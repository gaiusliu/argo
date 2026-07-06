package wake

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"argo/src/knot"
)

// LogWriter 实现 io.Writer，按日期+大小自动轮转日志文件。
// 日期优先：跨天立即切新文件；大小其次：同日超过上限追加序号 .001, .002...
type LogWriter struct {
	dir     string
	maxSize int64
	maxAge  int

	curDate string
	curSize int64
	seq     int

	file *os.File
}

// NewLogWriter 创建日志写入器：解析路径、创建目录、清理过期文件、打开当天日志文件。
func NewLogWriter(cfg knot.LogConfig) (*LogWriter, error) {
	// 解析目录：空则默认 ./logs/
	dir := cfg.Dir
	if dir == "" {
		dir = "./logs/"
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("wake: %w", err)
	}

	// 创建日志目录
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("wake: %w", err)
	}

	// 应用默认值并换算单位
	maxSize := int64(cfg.MaxSize)
	if maxSize <= 0 {
		maxSize = 100
	}
	maxSize *= 1024 * 1024 // MB → 字节
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = 7
	}

	w := &LogWriter{
		dir:     absDir,
		maxSize: maxSize,
		maxAge:  maxAge,
	}

	// 清理过期文件（失败不影响启动）
	_ = w.cleanup()

	// 打开当天日志文件
	if err := w.rotate(); err != nil {
		return nil, fmt.Errorf("wake: %w", err)
	}
	return w, nil
}

// Write 写入日志。先检查轮转条件（跨天 / 超大小），再执行写入。
func (w *LogWriter) Write(p []byte) (n int, err error) {
	// 检查是否触发轮转：跨天或即将超过大小上限
	today := time.Now().Format("2006-01-02")
	if today != w.curDate || w.curSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	// 写入并累加字节数
	n, err = w.file.Write(p)
	w.curSize += int64(n)
	return n, err
}

// rotate 关闭旧文件，按 "wake-日期[.NNN].log" 命名打开新文件。
func (w *LogWriter) rotate() error {
	// 关闭旧文件
	if w.file != nil {
		w.file.Close()
	}

	// 确定日期和序号
	today := time.Now().Format("2006-01-02")
	if w.curDate == "" {
		// 首次启动：扫描当天已有日志文件，取最大序号继续追加
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

	// 打开（或创建）日志文件，追加模式写入
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

// logPath 按日期和序号拼出日志文件路径
func (w *LogWriter) logPath() string {
	if w.seq == 0 {
		return filepath.Join(w.dir, "wake-"+w.curDate+".log")
	}
	return filepath.Join(w.dir, fmt.Sprintf("wake-%s.%03d.log", w.curDate, w.seq))
}

// findLatestSeq 扫描目录，返回当天日志文件的最大序号，无文件返回 -1
func findLatestSeq(dir, date string) int {
	prefix := "wake-" + date
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
		// 解析 .NNN.log：去掉前导句点和尾缀 .log，取三位数字
		if len(rest) == 8 && rest[0] == '.' && strings.HasSuffix(rest, ".log") {
			if n, err := strconv.Atoi(rest[1:4]); err == nil {
				maxSeq = max(maxSeq, n)
			}
		}
	}
	return maxSeq
}

// Close 关闭当前日志文件。
func (w *LogWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// cleanup 删除修改时间超过 maxAge 天的日志文件。
func (w *LogWriter) cleanup() error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -w.maxAge)

	for _, e := range entries {
		// 跳过目录和非日志文件
		if e.IsDir() || !strings.HasPrefix(e.Name(), "wake-") || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// 超过保留时限则删除
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(w.dir, e.Name()))
		}
	}
	return nil
}
