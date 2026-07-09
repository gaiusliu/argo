package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// startServer 确保 argo-server 在 addr 上运行，返回取消函数以关闭。
// 如果已有 server 在监听则复用，否则启动子进程并等待就绪。
func startServer(addr string) (context.CancelFunc, error) {
	if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		conn.Close()
		return func() {}, nil // 已有 server 运行，无需管理生命周期
	}

	// 查找 argo-server.exe：优先 exe 同目录，其次 PATH
	exePath := "argo-server"
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "argo-server.exe")); err == nil {
			exePath = filepath.Join(dir, "argo-server.exe")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("启动 argo-server 失败: %w", err)
	}

	// 轮询等待 server 就绪
	if err := waitReady(addr, 5*time.Second); err != nil {
		cancel()
		return nil, fmt.Errorf("argo-server 启动超时: %w", err)
	}

	return func() {
		cancel()
		cmd.Wait()
	}, nil
}

// waitReady 轮询 addr 直至有连接响应或超时。
func waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}
