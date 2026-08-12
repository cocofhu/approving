package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Official WeCom decryptFile pads to a 32-byte multiple (padLen may be 1–32).
const pkcs7MaxPad = 32

// DecryptImageAES256CBC decrypts a WeCom AI-bot private-chat image.
// Official: AES-256-CBC, PKCS#7 (pad up to 32), IV = first 16 bytes of the
// Base64-decoded aeskey (URL-safe, often 43 chars without padding).
func DecryptImageAES256CBC(ciphertext []byte, aeskey string) ([]byte, error) {
	key, err := decodeAESKey(aeskey)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("wecom: aeskey 解码后长度 %d，期望 32", len(key))
	}
	if len(ciphertext) < aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("wecom: 密文长度非法")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := key[:16]
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	return pkcs7Unpad(plain, pkcs7MaxPad)
}

func decodeAESKey(aeskey string) ([]byte, error) {
	raw := strings.TrimSpace(aeskey)
	if raw == "" {
		return nil, fmt.Errorf("wecom: aeskey 为空")
	}
	// Official SDK: StdEncoding / URLEncoding Base64 (pad if needed) → 32 bytes.
	if decoded, ok := decodeBase64AESKey(raw); ok {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("wecom: 无法解析 aeskey")
}

func decodeBase64AESKey(raw string) ([]byte, bool) {
	padded := padBase64(raw)
	for _, dec := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	} {
		out, err := dec(padded)
		if err != nil {
			out, err = dec(raw)
		}
		if err == nil && len(out) == 32 {
			return out, true
		}
	}
	return nil, false
}

func padBase64(s string) string {
	if m := len(s) % 4; m != 0 {
		return s + strings.Repeat("=", 4-m)
	}
	return s
}

func pkcs7Unpad(data []byte, maxPad int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("wecom: PKCS#7 填充非法")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > maxPad || pad > len(data) {
		return nil, fmt.Errorf("wecom: PKCS#7 填充长度非法")
	}
	for i := 0; i < pad; i++ {
		if data[len(data)-1-i] != byte(pad) {
			return nil, fmt.Errorf("wecom: PKCS#7 填充内容非法")
		}
	}
	return data[:len(data)-pad], nil
}
