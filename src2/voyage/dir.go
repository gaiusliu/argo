package voyage

import (
	"os"
	"path/filepath"
)

// argoHome 返回 Argo 数据目录（优先 $ARGO_HOME，否则 ~/.argo/），并确保目录存在。
func argoHome() string {
	var dir string
	if d := os.Getenv("ARGO_HOME"); d != "" {
		dir = filepath.Clean(d)
	} else {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".argo")
	}
	_ = os.MkdirAll(dir, 0755)
	return dir
}
