package agent

import (
	"strings"

	"github.com/smallnest/goclaw/types"
)

// For backwards compatibility, re-export the types
type FailoverReason = types.FailoverReason

const (
	FailoverReasonAuth      FailoverReason = types.FailoverReasonAuth
	FailoverReasonRateLimit FailoverReason = types.FailoverReasonRateLimit
	FailoverReasonTimeout   FailoverReason = types.FailoverReasonTimeout
	FailoverReasonBilling   FailoverReason = types.FailoverReasonBilling
	FailoverReasonUnknown   FailoverReason = types.FailoverReasonUnknown
)

// ErrorClassifier 错误分类器
type ErrorClassifier struct {
	// 认证错误模式
	authPatterns []string
	// 限流错误模式
	rateLimitPatterns []string
	// 超时错误模式
	timeoutPatterns []string
	// 计费错误模式
	billingPatterns []string
}

// Ensure ErrorClassifier implements types.ErrorClassifier
var _ types.ErrorClassifier = (*ErrorClassifier)(nil)

// NewErrorClassifier 创建错误分类器
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{
		authPatterns: []string{
			"invalid api key",
			"incorrect api key",
			"invalid token",
			"authentication",
			"re-authenticate",
			"oauth token refresh failed",
			"unauthorized",
			"forbidden",
			"access denied",
			"expired",
			"token has expired",
			"401",
			"403",
			"no credentials found",
			"no api key found",
		},
		rateLimitPatterns: []string{
			"rate limit",
			"too many requests",
			"429",
			"exceeded your current quota",
			"resource has been exhausted",
			"quota exceeded",
			"resource_exhausted",
			"usage limit",
			"overloaded",
		},
		timeoutPatterns: []string{
			"timeout",
			"timed out",
			"deadline exceeded",
			"context deadline exceeded",
		},
		billingPatterns: []string{
			"402",
			"payment required",
			"insufficient credits",
			"credit balance",
			"plans & billing",
			"billing",
		},
	}
}

// ClassifyError 分类错误
func (c *ErrorClassifier) ClassifyError(err error) FailoverReason {
	if err == nil {
		return FailoverReasonUnknown
	}

	errMsg := strings.ToLower(err.Error())

	// 按优先级检查错误模式
	if c.matchesAny(errMsg, c.authPatterns) {
		return FailoverReasonAuth
	}
	if c.matchesAny(errMsg, c.rateLimitPatterns) {
		return FailoverReasonRateLimit
	}
	if c.matchesAny(errMsg, c.timeoutPatterns) {
		return FailoverReasonTimeout
	}
	if c.matchesAny(errMsg, c.billingPatterns) {
		return FailoverReasonBilling
	}

	return FailoverReasonUnknown
}

// matchesAny 检查错误消息是否匹配任何模式
func (c *ErrorClassifier) matchesAny(errMsg string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

// IsContextOverflowError 检查是否为上下文溢出错误
func IsContextOverflowError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	hasRequestSizeExceeds := strings.Contains(lower, "request size exceeds")
	hasContextWindow := strings.Contains(lower, "context window") ||
		strings.Contains(lower, "context length") ||
		strings.Contains(lower, "maximum context length")

	return strings.Contains(lower, "request_too_large") ||
		strings.Contains(lower, "request exceeds the maximum size") ||
		strings.Contains(lower, "context length exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "prompt is too long") ||
		strings.Contains(lower, "exceeds model context window") ||
		(hasRequestSizeExceeds && hasContextWindow) ||
		strings.Contains(lower, "context overflow") ||
		(strings.Contains(lower, "413") && strings.Contains(lower, "too large"))
}

// IsRoleOrderingError 检查是否为角色顺序错误
func IsRoleOrderingError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "incorrect role information") ||
		strings.Contains(lower, "roles must alternate")
}

// IsImageSizeError 检查是否为图像大小错误
func IsImageSizeError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "image exceeds") &&
		strings.Contains(lower, "mb")
}

// IsFailoverError 检查是否为可回退的错误
func (c *ErrorClassifier) IsFailoverError(err error) bool {
	if err == nil {
		return false
	}
	reason := c.ClassifyError(err)
	return reason != FailoverReasonUnknown
}

// FormatErrorForUser 格式化错误信息供用户查看
func FormatErrorForUser(errMsg string) string {
	if errMsg == "" {
		return "An unknown error occurred."
	}

	trimmed := strings.TrimSpace(errMsg)

	// 上下文溢出
	if IsContextOverflowError(trimmed) {
		return "Context overflow: prompt too large for the model. Try again with less input or a larger-context model."
	}

	// 角色顺序错误
	if IsRoleOrderingError(trimmed) {
		return "Message ordering conflict - please try again. If this persists, use /new to start a fresh session."
	}

	// 图像大小错误
	if IsImageSizeError(trimmed) {
		return "Image too large for the model. Please compress or resize the image and try again."
	}

	// 限制长度
	if len(trimmed) > 600 {
		return trimmed[:600] + "…"
	}

	return trimmed
}
