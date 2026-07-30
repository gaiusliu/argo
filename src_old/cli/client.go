package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// argoClient 与 argo-server 通信的 HTTP 客户端。
type argoClient struct {
	addr   string
	client *http.Client
}

func newArgoClient(addr string) *argoClient {
	return &argoClient{addr: addr, client: &http.Client{}}
}

// postJSON 发送 POST 请求，解析 JSON 响应到 v。
func (c *argoClient) postJSON(path string, body any, v any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(data)
	}
	resp, err := c.client.Post("http://"+c.addr+path, "application/json", r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// getJSON 发送 GET 请求，解析 JSON 响应到 v。
func (c *argoClient) getJSON(path string, v any) error {
	resp, err := c.client.Get("http://" + c.addr + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
