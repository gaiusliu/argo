package truncation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ==================== Write 测试 ====================

// TestWriter_Write_Success 测试正常写入场景。
// 目的：验证 Write 返回绝对路径，文件存在且内容等于原始 content。
// 方法：创建临时目录 writer → Write("hello world") → 检查返回路径、文件存在性、内容匹配。
// 预期：path 非空且为绝对路径，err 为 nil，文件内容为 "hello world"。
func TestWriter_Write_Success(t *testing.T) {
	w := newWriter(t.TempDir())

	path, err := w.Write("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path, got empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}

	// 验证文件存在且内容正确
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", string(data), "hello world")
	}
}

// TestWriter_Write_EmptyContent 测试空字符串写入场景。
// 目的：验证空 content 也能正常写入，返回有效路径。
// 方法：Write("") → 检查返回路径非空，文件内容为空。
// 预期：path 非空，err 为 nil，文件存在且内容长度为 0。
func TestWriter_Write_EmptyContent(t *testing.T) {
	w := newWriter(t.TempDir())

	path, err := w.Write("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path for empty content, got empty string")
	}

	// 验证文件存在且为空
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty file, got %q", string(data))
	}
}

// TestWriter_Write_UniquePath 测试多次写入路径唯一性。
// 目的：验证连续 Write 返回不同路径（unix-nano + random 兜底保证唯一性）。
// 方法：连续两次 Write → 比较返回的绝对路径。
// 预期：两次返回的 path 不同。
func TestWriter_Write_UniquePath(t *testing.T) {
	w := newWriter(t.TempDir())

	path1, err1 := w.Write("first")
	if err1 != nil {
		t.Fatalf("first write error: %v", err1)
	}
	path2, err2 := w.Write("second")
	if err2 != nil {
		t.Fatalf("second write error: %v", err2)
	}

	if path1 == path2 {
		t.Errorf("expected unique paths, but both returned %q", path1)
	}
}

// TestWriter_Write_DirectoryNotExist 测试目录自动创建场景。
// 目的：验证目录不存在时自动创建（os.MkdirAll）。
// 方法：writer 指向不存在的多级子目录 → Write → 检查目录和文件是否被创建。
// 预期：目录被自动创建，文件成功写入且存在。
func TestWriter_Write_DirectoryNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "nonexistent", "subdir")
	w := newWriter(subDir)

	path, err := w.Write("content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	// 验证目录被创建
	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected %q to be a directory", subDir)
	}

	// 验证文件存在
	if _, err := os.Stat(path); err != nil {
		t.Errorf("written file does not exist: %v", err)
	}
}

// TestWriter_Write_Failure 测试 IO 错误场景。
// 目的：模拟目录创建失败的错误，验证 Write 返回 err 且 path="" 且不 panic。
// 方法：创建一个普通文件挡住目录路径 → Write → 检查错误返回。
// 预期：err != nil，path == ""。
func TestWriter_Write_Failure(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个普通文件作为"障碍物"，阻止目录创建
	blockerFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockerFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// 让 writer 的目录路径穿过这个文件（文件不是目录，MkdirAll 会失败）
	w := newWriter(filepath.Join(blockerFile, "subdir"))

	path, err := w.Write("content")
	if err == nil {
		t.Error("expected error when directory creation is blocked, got nil")
	}
	if path != "" {
		t.Errorf("expected empty path on failure, got %q", path)
	}
}

// ==================== 辅助函数 ====================

// createFileWithMtime 创建文件并设置其修改时间（用于清理测试）。
func createFileWithMtime(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file %q: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("failed to set mtime on %q: %v", path, err)
	}
}

// countLogFiles 统计目录下匹配 tool-*.log 的文件数量。
func countLogFiles(t *testing.T, dir string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "tool-*.log"))
	if err != nil {
		t.Fatalf("glob failed in %q: %v", dir, err)
	}
	return len(matches)
}

// ==================== Cleanup 测试 ====================

// TestCleanup_DeletesOldFiles 测试清理过期文件场景。
// 目的：验证 mtime 超出 retention 的文件被删除。
// 方法：创建 3 个过期文件（30 天前）→ 启动清理 → 等待 → 检查文件被删除。
// 预期：过期文件全部被删除（tool-*.log 数量为 0）。
func TestCleanup_DeletesOldFiles(t *testing.T) {
	dir := t.TempDir()
	w := newWriter(dir)
	os.MkdirAll(dir, 0755)

	// 创建 3 个过期文件（30 天前）
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	for i := 0; i < 3; i++ {
		fname := filepath.Join(dir, "tool-old-"+string(rune('a'+i))+".log")
		createFileWithMtime(t, fname, "old content", oldTime)
	}

	// 启动清理：retention=7天，interval=10ms
	stop, ticked := StartCleanup(w, 7*24*time.Hour, 10*time.Millisecond)
	defer stop()

	// 通过 channel 同步等待清理完成，消除 flake 风险
	<-ticked

	// 过期文件应被删除
	if n := countLogFiles(t, dir); n != 0 {
		t.Errorf("expected 0 log files after cleanup, got %d", n)
	}
}

// TestCleanup_KeepsRecentFiles 测试保留近期文件场景。
// 目的：验证 mtime 未超出 retention 的文件被保留，同时过期文件被删除。
// 方法：混合创建 2 个过期 + 1 个近期文件 → 启动清理 → 等待 → 验证仅近期文件保留。
// 预期：过期文件被删除，近期文件保留（tool-*.log 数量 = 1）。
func TestCleanup_KeepsRecentFiles(t *testing.T) {
	dir := t.TempDir()
	w := newWriter(dir)
	os.MkdirAll(dir, 0755)

	// 创建 2 个过期文件（30 天前）
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	createFileWithMtime(t, filepath.Join(dir, "tool-old-a.log"), "old a", oldTime)
	createFileWithMtime(t, filepath.Join(dir, "tool-old-b.log"), "old b", oldTime)

	// 创建 1 个近期文件（1 小时前，在 7 天 retention 内）
	recentTime := time.Now().Add(-1 * time.Hour)
	createFileWithMtime(t, filepath.Join(dir, "tool-recent.log"), "recent", recentTime)

	// 启动清理：retention=7天，interval=10ms
	stop, ticked := StartCleanup(w, 7*24*time.Hour, 10*time.Millisecond)
	defer stop()

	// 通过 channel 同步等待清理完成
	<-ticked

	// 仅近期文件应保留（过期 2 个被删除）
	if n := countLogFiles(t, dir); n != 1 {
		t.Errorf("expected 1 recent file after cleanup (2 old deleted), got %d", n)
	}
}

// TestCleanup_Stop 测试停止清理场景。
// 目的：验证 stop() 后 ticker 停止，goroutine 不再清理文件。
// 方法：启动清理 → 等待第一批过期文件被清理 → stop() → 创建新过期文件 → 等待 → 验证新文件未被清理。
// 预期：stop() 前过期文件被删除，stop() 后新过期文件保留。
func TestCleanup_Stop(t *testing.T) {
	dir := t.TempDir()
	w := newWriter(dir)
	os.MkdirAll(dir, 0755)

	// 第一批：创建过期文件（30 天前）
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	oldFile1 := filepath.Join(dir, "tool-batch1.log")
	createFileWithMtime(t, oldFile1, "batch1", oldTime)

	// 启动清理：retention=7天，interval=10ms
	stop, ticked := StartCleanup(w, 7*24*time.Hour, 10*time.Millisecond)

	// 通过 channel 同步等待第一轮清理完成
	<-ticked

	if _, err := os.Stat(oldFile1); err == nil {
		t.Error("expected batch1 file to be deleted before stop, but it still exists")
	}

	// 停止清理
	stop()

	// 第二批：创建新的过期文件
	oldFile2 := filepath.Join(dir, "tool-batch2.log")
	createFileWithMtime(t, oldFile2, "batch2", oldTime)

	// stop() 后通道已关闭，需用 sleep 确认 goroutine 已退出
	time.Sleep(50 * time.Millisecond)

	// 第二批文件应保留（清理已停止，不再删除）
	if _, err := os.Stat(oldFile2); err != nil {
		t.Errorf("expected batch2 file to remain after stop, but got error: %v", err)
	}
}
