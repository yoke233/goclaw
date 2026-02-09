package types

import (
	"strings"
)

// FailoverReason 失败原因类型
type FailoverReason string

const (
	// FailoverReasonAuth 认证错误
	FailoverReasonAuth FailoverReason = "auth"
	// FailoverReasonRateLimit 速率限制
	FailoverReasonRateLimit FailoverReason = "rate_limit"
	// FailoverReasonTimeout 超时
	FailoverReasonTimeout FailoverReason = "timeout"
	// FailoverReasonBilling 计费错误
	FailoverReasonBilling FailoverReason = "billing"
	// FailoverReasonContextOverflow 上下文溢出
	FailoverReasonContextOverflow FailoverReason = "context_overflow"
	// FailoverReasonUnknown 未知错误
	FailoverReasonUnknown FailoverReason = "unknown"
)

// ErrorClassifier 错误分类器接口
type ErrorClassifier interface {
	ClassifyError(err error) FailoverReason
	IsFailoverError(err error) bool
}

// SimpleErrorClassifier 简单的错误分类器实现
type SimpleErrorClassifier struct {
	authPatterns      []string
	rateLimitPatterns []string
	timeoutPatterns   []string
	billingPatterns   []string
}

// NewSimpleErrorClassifier 创建简单错误分类器
func NewSimpleErrorClassifier() *SimpleErrorClassifier {
	return &SimpleErrorClassifier{
		authPatterns: []string{
			"invalid api key", "incorrect api key", "invalid token",
			"authentication", "re-authenticate", "unauthorized",
			"forbidden", "access denied", "expired", "401", "403",
		},
		rateLimitPatterns: []string{
			"rate limit", "too many requests", "429", "quota exceeded",
			"resource_exhausted", "usage limit", "overloaded",
		},
		timeoutPatterns: []string{
			"timeout", "timed out", "deadline exceeded", "context deadline exceeded",
		},
		billingPatterns: []string{
			"402", "payment required", "insufficient credits", "billing",
		},
	}
}

// ClassifyError 分类错误
func (c *SimpleErrorClassifier) ClassifyError(err error) FailoverReason {
	if err == nil {
		return FailoverReasonUnknown
	}

	errMsg := strings.ToLower(err.Error())

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

// IsFailoverError 检查是否为可回退的错误
func (c *SimpleErrorClassifier) IsFailoverError(err error) bool {
	if err == nil {
		return false
	}
	reason := c.ClassifyError(err)
	return reason != FailoverReasonUnknown
}

// matchesAny 检查错误消息是否匹配任何模式
func (c *SimpleErrorClassifier) matchesAny(errMsg string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}
