package vault

import "os"

// NewFileJournal 创建基于文件的 Journal，每行一条 JSON 记录。
func NewFileJournal(path string) Journal {
	return &fileJournal{path: path}
}

type fileJournal struct {
	path string
}

func (j *fileJournal) Append(records []string) error {
	f, err := os.OpenFile(j.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, rec := range records {
		if _, err := f.WriteString(rec + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (j *fileJournal) Scan() ([]string, error) {
	data, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// 按换行切分，跳过尾行空串
	lines := splitLines(string(data))
	return lines, nil
}

// splitLines 按 '\n' 切分字符串，跳过末尾空行。
func splitLines(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	return result
}
