package sail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// APIError 分类后的 API 错误，helm 据此决定重试或通知用户
type APIError struct {
	StatusCode         int
	Code               string // auth / quota / rate_limit / server_error / context_overflow / network
	Retryable          bool
	RetryAfterSeconds  int
	Message            string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sail: %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
}

// classifyError 按 HTTP 状态码 + 响应体 + 响应头分类
func classifyError(statusCode int, body []byte, header http.Header) *APIError {
	ae := &APIError{StatusCode: statusCode}

	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		ae.Code = "auth"
		ae.Retryable = false
		ae.Message = "API Key 无效或权限不足，请检查配置"

	case statusCode == http.StatusRequestEntityTooLarge:
		ae.Code = "context_overflow"
		ae.Retryable = false
		ae.Message = "上下文超出模型窗口，需触发压缩"

	case statusCode == http.StatusTooManyRequests:
		// 提取 Retry-After
		if ra := header.Get("Retry-After"); ra != "" {
			if sec, err := strconv.Atoi(ra); err == nil {
				ae.RetryAfterSeconds = sec
			}
		}
		// 检查响应体：billing/quota 耗尽也算 429，但不可重试
		ae.Code = "rate_limit"
		ae.Retryable = true
		ae.Message = "请求频率过高，等待后重试"
		if msg := parseOpenAIError(body); msg != "" {
			ae.Message = msg
			if strings.Contains(strings.ToLower(msg), "quota") ||
				strings.Contains(strings.ToLower(msg), "billing") ||
				strings.Contains(strings.ToLower(msg), "insufficient") {
				ae.Code = "quota"
				ae.Retryable = false
			}
		}

	case statusCode == http.StatusPaymentRequired:
		ae.Code = "quota"
		ae.Retryable = false
		ae.Message = "余额不足或配额用尽，请检查账户"

	case statusCode >= 500:
		ae.Code = "server_error"
		ae.Retryable = true
		ae.Message = "服务端临时故障"

	default:
		ae.Code = "server_error"
		ae.Retryable = true
		ae.Message = string(body)
	}

	return ae
}

// parseOpenAIError 从 OpenAI 格式的错误响应中提取 message
func parseOpenAIError(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return ""
}
