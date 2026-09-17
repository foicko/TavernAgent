package http

import (
	crand "crypto/rand"
	"crypto/subtle"
	"encoding/hex"
)

// generateRandomToken 生成 n 字节的随机 Token 十六进制串；crypto/rand 失败时返回错误，
// 由调用方在启动阶段决定如何处理（不再 panic）。
func generateRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// secureEqual 以恒时比较两个密钥，避免通过响应时间侧信道推断配对码或 Token。
// 长度不同会立即返回 false——长度不是秘密：PIN 固定 6 位、Token 固定 48 个十六进制字符。
func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
