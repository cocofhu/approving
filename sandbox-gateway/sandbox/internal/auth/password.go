package auth

import (
	"crypto/sha256"
	"crypto/subtle"
)

// PasswordMatch 以常量时间比较口令（长度不泄露给计时侧信道）。
func PasswordMatch(expected, given string) bool {
	eh := sha256.Sum256([]byte(expected))
	gh := sha256.Sum256([]byte(given))
	return subtle.ConstantTimeCompare(eh[:], gh[:]) == 1
}
