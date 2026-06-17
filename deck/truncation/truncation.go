// Package truncation 提供工具输出截断前的完整内容存储与过期清理能力。
package truncation

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Writer 抽象 truncation 目录的文件写入能力。
type Writer interface {
	// Dir 返回 Writer 关联的存储目录绝对路径。
	Dir() string
	// Write 把 content 写入 truncation 目录，返回完整绝对路径。
	// 失败时返回 ("", error)，不 panic。
	Write(content string) (path string, err error)
}

// defaultWriter 默认 Writer 实现，持有目标目录路径。
type defaultWriter struct {
	dir string
}

// Write 将 content 写入 truncation 目录，文件名格式 tool-<unix-nano>-<random>.log。
// 若目录不存在则自动创建（os.MkdirAll 权限 0755，允许用户间共享查看），写入成功返回文件绝对路径，失败返回 ("", err)。
func (dw *defaultWriter) Write(content string) (string, error) {
	// 生成 8 字节随机后缀，hex 编码保证文件名唯一性
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}

	// 构造文件名：tool-<unix-nano>-<random>.log
	filename := fmt.Sprintf("tool-%d-%x.log", time.Now().UnixNano(), randBytes)
	filePath := filepath.Join(dw.dir, filename)

	// 确保目录存在（含多级子目录，0755 支持多用户共享查看）
	if err := os.MkdirAll(dw.dir, 0755); err != nil {
		return "", err
	}

	// 写文件（权限 0644）
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", err
	}

	// dw.dir 为绝对路径（NewDefault 或 t.TempDir），Join 产出即为绝对路径
	return filePath, nil
}

// Dir 返回存储目录绝对路径。
func (dw *defaultWriter) Dir() string {
	return dw.dir
}

// NewDefault 返回默认 Writer，写入 ~/.argo/truncation/ 目录，不预创建目录。
func NewDefault() Writer {
	home, err := os.UserHomeDir()
	if err != nil {
		Logf("truncation: UserHomeDir failed, falling back to os.TempDir: %v", err)
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".argo", "truncation")
	return &defaultWriter{dir: dir}
}

// newWriter 创建写入指定目录的 Writer（用于测试注入）。
func newWriter(dir string) Writer {
	return &defaultWriter{dir: dir}
}

// Logf 是清理日志记录函数，默认使用 log.Printf。
// 调用方可替换为自己的 logger。
var Logf = log.Printf

// StartCleanup 启动后台 ticker 清理超期文件。
// retention: 文件保留期，interval: 清理检查间隔。
// 返回 stop() 停止清理 + ticked 通道（每次清理 tick 完成后发送信号，stop 后关闭）。
func StartCleanup(w Writer, retention time.Duration, interval time.Duration) (stop func(), _ <-chan struct{}) {
	ticked := make(chan struct{}, 1)     // 缓冲通道，避免清理发送阻塞
	ticker := time.NewTicker(interval) // 定时器，按 interval 触发清理
	done := make(chan struct{})         // 停止信号通道
	var stopOnce sync.Once             // 保证 stop() 幂等

	go func() {
		defer ticker.Stop()
		defer close(ticked) // goroutine 退出时关闭 ticked 通道

		for {
			select {
			case <-ticker.C:
				// 扫描并清理超期文件
				scanAndClean(w, retention)

				// 非阻塞发送 tick 完成信号
				select {
				case ticked <- struct{}{}:
				default:
				}
			case <-done:
				return // 收到停止信号，退出 goroutine
			}
		}
	}()

	stop = func() {
		stopOnce.Do(func() {
			ticker.Stop() // 停止定时器
			close(done)   // 通知 goroutine 退出
		})
	}

	return stop, ticked
}

// scanAndClean 扫描 w.Dir() 下匹配 tool-*.log 的文件，删除 mtime 超出 retention 的过期文件。
// 单个文件删除失败时通过 Logf 记录日志，继续处理其余文件（best-effort）。
func scanAndClean(w Writer, retention time.Duration) {
	dir := w.Dir()

	// 扫描目录下所有 tool-*.log 文件
	pattern := filepath.Join(dir, "tool-*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		// 目录不存在或无权限时 Glob 可能返回 err，静默返回
		return
	}

	now := time.Now()
	for _, file := range matches {
		info, err := os.Stat(file)
		if err != nil {
			// 文件可能已在 Glob 和 Stat 之间被删除，跳过
			continue
		}

		// 文件修改时间距今超过 retention 则删除
		if now.Sub(info.ModTime()) > retention {
			if err := os.Remove(file); err != nil {
				Logf("truncation cleanup: failed to remove %s: %v", file, err)
			}
		}
	}
}
