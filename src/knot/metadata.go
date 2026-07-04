package knot

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
)

// 全局 metadata 缓存
var (
	metadataMu    sync.RWMutex
	metadataCache map[string]int // modelID → contextWindow
	metadataOnce  sync.Once
)

// 解析 models.dev API 响应的局部类型
type modelsDevResponse map[string]struct {
	Models map[string]struct {
		Name  string `json:"name"`
		Limit struct {
			Context int `json:"context"`
		} `json:"limit"`
	} `json:"models"`
}

// metadataPath 返回 ~/.argo/metadata.json
func metadataPath() (string, error) {
	if home := os.Getenv("ARGO_HOME"); home != "" {
		return home + "/metadata.json", nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return dir + "/.argo/metadata.json", nil
}

// fetchMetadata 从 models.dev 拉取所有模型的 context window 数据
func fetchMetadata() (map[string]int, error) {
	resp, err := http.Get("https://models.dev/api.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var data modelsDevResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	// 展平为 provider_id/model_id → context window
	result := make(map[string]int)
	for _, p := range data {
		for modelID, m := range p.Models {
			if m.Limit.Context > 0 {
				result[modelID] = m.Limit.Context
			}
		}
	}
	return result, nil
}

// saveMetadata 将 metadata map 写入本地 JSON 文件
func saveMetadata(data map[string]int) error {
	path, err := metadataPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(data)
}

// loadMetadata 从本地 JSON 文件加载缓存的 metadata
func loadMetadata() (map[string]int, error) {
	path, err := metadataPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var data map[string]int
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// ensureMetadata 懒加载 metadata——优先本地缓存，后台异步拉取最新数据
func ensureMetadata() {
	metadataOnce.Do(func() {
		// 先加载本地缓存，快速可用
		if data, err := loadMetadata(); err == nil {
			metadataMu.Lock()
			metadataCache = data
			metadataMu.Unlock()
		}
		// 后台异步拉取最新数据
		go func() {
			data, err := fetchMetadata()
			if err != nil {
				return
			}
			saveMetadata(data)
			metadataMu.Lock()
			metadataCache = data
			metadataMu.Unlock()
		}()
	})
}

// LookupWindow 从内置数据表查找模型的 context window token 上限。
// 返回 0 表示未命中，调用方应使用保守默认值。
func LookupWindow(modelName string) int {
	ensureMetadata()
	metadataMu.RLock()
	defer metadataMu.RUnlock()

	if metadataCache == nil {
		return 0
	}
	// 精确匹配
	if v, ok := metadataCache[modelName]; ok {
		return v
	}
	// 前缀匹配 model 部分：取 / 后面 → 加版本分隔符边界（- 或 .）
	for id, v := range metadataCache {
		idx := len(id) - 1
		for idx >= 0 && id[idx] != '/' {
			idx--
		}
		modelPart := id[idx+1:]
		if len(modelPart) >= len(modelName) &&
			modelPart[:len(modelName)] == modelName &&
			(len(modelPart) == len(modelName) || modelPart[len(modelName)] == '-' || modelPart[len(modelName)] == '.') {
			return v
		}
	}
	return 0
}
