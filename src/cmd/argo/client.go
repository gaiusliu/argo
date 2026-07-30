// Package main Argo CLI 客户端。
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Client 封装 /voyage/* 和 /prompt HTTP 调用。
type Client struct {
	addr   string
	client *http.Client
}

// NewClient 创建 CLI 客户端。
func NewClient(addr string) *Client {
	return &Client{addr: addr, client: &http.Client{}}
}

// VoyageInfo 服务端返回的 voyage 元数据。
type VoyageInfo struct {
	ID           string `json:"id"`
	CreatedAt    string `json:"createdAt"`
	LastActiveAt string `json:"lastActiveAt"`
	Name         string `json:"name,omitempty"`
}

func (c *Client) postJSON(path string, body, dst any) error {
	data, _ := json.Marshal(body)
	resp, err := c.client.Post("http://"+c.addr+path, "application/json",
		bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("post %s: status %d", path, resp.StatusCode)
	}
	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

// CreateVoyage 创建新 voyage，返回 ID。
func (c *Client) CreateVoyage() (string, error) {
	var resp struct {
		VoyageID string `json:"voyageID"`
	}
	if err := c.postJSON("/voyage/new", nil, &resp); err != nil {
		return "", err
	}
	return resp.VoyageID, nil
}

// ListVoyages 获取 voyage 列表。
func (c *Client) ListVoyages() ([]VoyageInfo, error) {
	resp, err := c.client.Get("http://" + c.addr + "/voyage/list")
	if err != nil {
		return nil, fmt.Errorf("list voyages: %w", err)
	}
	defer resp.Body.Close()
	var infos []VoyageInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		return nil, err
	}
	return infos, nil
}

// DeleteVoyage 删除 voyage。
func (c *Client) DeleteVoyage(voyageID string) error {
	return c.postJSON("/voyage/delete", map[string]string{
		"voyageID": voyageID,
	}, nil)
}

// ResumeResponse 恢复 voyage 的响应。
type ResumeResponse struct {
	Info     VoyageInfo        `json:"info"`
	Messages []json.RawMessage `json:"messages"`
}

// ResumeVoyage 获取 voyage 的历史消息。
func (c *Client) ResumeVoyage(voyageID string) (*ResumeResponse, error) {
	var resp ResumeResponse
	if err := c.postJSON("/voyage/resume", map[string]string{
		"voyageID": voyageID,
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RenameVoyage 重命名 voyage。
func (c *Client) RenameVoyage(voyageID, name string) error {
	return c.postJSON("/voyage/rename", map[string]string{
		"voyageID": voyageID,
		"name":     name,
	}, nil)
}

// SSEEvent 单条 SSE 事件。
type SSEEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
	Omens []Omen `json:"omens,omitempty"`
	Err   string `json:"error,omitempty"`
}

// Omen 工具调用信息。
type Omen struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Verdict   int    `json:"verdict"`
}

// Prompt 发送 prompt 或审批请求，返回 SSE 事件 channel。
func (c *Client) Prompt(voyageID, prompt string, decisions map[string]int) (<-chan SSEEvent, error) {
	body := map[string]any{"voyageID": voyageID}
	if len(decisions) > 0 {
		body["decisions"] = decisions
	} else {
		body["prompt"] = prompt
	}
	data, _ := json.Marshal(body)
	resp, err := c.client.Post("http://"+c.addr+"/prompt", "application/json",
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("prompt: %w", err)
	}
	ch := make(chan SSEEvent, 8)
	go parseSSE(resp, ch)
	return ch, nil
}

// parseSSE 从 HTTP 响应解析 SSE 事件流。
func parseSSE(resp *http.Response, ch chan<- SSEEvent) {
	defer close(ch)
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	var data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			continue
		}
		if line == "" && data != "" {
			var ev SSEEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				ch <- ev
			}
			data = ""
		}
	}
}
