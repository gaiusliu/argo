package vault

import "os"

// NewFileCheckpoint 创建基于文件的 Checkpoint。
func NewFileCheckpoint(path string) Checkpoint {
	return &fileCheckpoint{path: path}
}

type fileCheckpoint struct {
	path string
}

func (c *fileCheckpoint) Load() (string, error) {
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *fileCheckpoint) Save(data string) error {
	return os.WriteFile(c.path, []byte(data), 0644)
}

func (c *fileCheckpoint) Delete() error {
	err := os.Remove(c.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
