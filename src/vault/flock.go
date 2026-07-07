package vault

import (
	"os"
	"time"
)

// LockDir 对目标路径加目录锁，返回锁目录路径。
// 使用 os.Mkdir 原子操作，对标 opencode 的 Flock 实现。零外部依赖，全平台可用。
// 调用方必须在完成后调用 UnlockDir 释放锁。
func LockDir(targetPath string) (string, error) {
	lockPath := targetPath + ".lock"
	delay := 100 * time.Millisecond

	for {
		err := os.Mkdir(lockPath, 0700)
		if err == nil {
			return lockPath, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
		// time.Sleep 会让出 goroutine，不是自旋
		time.Sleep(delay)
		if delay < time.Second {
			delay *= 2
		}
	}
}

// UnlockDir 释放目录锁。
func UnlockDir(lockPath string) error {
	return os.Remove(lockPath)
}
