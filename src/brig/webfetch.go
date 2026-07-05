package brig

import "net/url"

// extractDomain 从 URL 字符串中提取域名（host），作为 web_fetch 审批的匹配 pattern。
// 如 "https://api.github.com/repos/..." → "api.github.com"。
// 解析失败返回空字符串，调用方视为无需审批。
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
