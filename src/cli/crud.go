package main

import "argo/src/server"

// createSession 发送创建会话请求，返回 sessionID。
func (c *argoClient) createSession(cwd string) (string, error) {
	var resp server.NewSessionResponse
	req := server.NewSessionRequest{CWD: cwd}
	if err := c.postJSON("/session/new", req, &resp); err != nil {
		return "", err
	}
	return resp.SessionID, nil
}

// listSessions 获取所有已持久化的会话列表。
func (c *argoClient) listSessions() ([]server.SessionInfoJSON, error) {
	var resp server.ListSessionsResponse
	if err := c.getJSON("/session/list", &resp); err != nil {
		return nil, err
	}
	return resp.SessionInfos, nil
}

// resumeSession 恢复指定会话，返回其元数据。
func (c *argoClient) resumeSession(id string) (*server.GetSessionResponse, error) {
	var resp server.GetSessionResponse
	req := server.ResumeSessionRequest{SessionID: id}
	if err := c.postJSON("/session/resume", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// deleteSession 删除指定会话。
func (c *argoClient) deleteSession(id string) error {
	req := server.DeleteSessionRequest{SessionID: id}
	return c.postJSON("/session/delete", req, nil)
}
