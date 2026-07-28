package sight

import "net/http"

// APIError 分类后的 API 错误，调用方据此决定重试策略。
type APIError struct {
	// Code 错误分类：auth / quota / rate_limit / server_error / context_overflow / model_not_found
	Code string
	// Message 错误详情
	Message string
}

func (e *APIError) Error() string {
	return "provider: " + e.Code + ": " + e.Message
}

// classifyError 按 HTTP 状态码分类错误。
func classifyError(statusCode int, body []byte, _ http.Header) *APIError {
	ae := &APIError{}
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		ae.Code = "auth"
		ae.Message = "API Key 无效或权限不足"
	case statusCode == http.StatusRequestEntityTooLarge:
		ae.Code = "context_overflow"
		ae.Message = "上下文超出窗口，需触发压缩"
	case statusCode == http.StatusTooManyRequests:
		ae.Code = "rate_limit"
		ae.Message = "请求频率过高"
	case statusCode == http.StatusPaymentRequired:
		ae.Code = "quota"
		ae.Message = "余额不足或配额用尽"
	case statusCode >= 500:
		ae.Code = "server_error"
		ae.Message = "服务端临时故障"
	default:
		ae.Code = "server_error"
		ae.Message = string(body)
	}
	return ae
}
