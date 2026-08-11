package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"strings"
)

// DecryptImageAES256CBC decrypts a WeCom AI-bot private-chat image.
// Official: AES-256-CBC, PKCS#7, IV = first 16 bytes of the decoded aeskey.
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
	return pkcs7Unpad(plain, aes.BlockSize)
}

func decodeAESKey(aeskey string) ([]byte, error) {
	raw := strings.TrimSpace(aeskey)
	if raw == "" {
		return nil, fmt.Errorf("wecom: aeskey 为空")
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("wecom: 无法解析 aeskey")
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("wecom: PKCS#7 填充非法")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize || pad > len(data) {
		return nil, fmt.Errorf("wecom: PKCS#7 填充长度非法")
	}
	for i := 0; i < pad; i++ {
		if data[len(data)-1-i] != byte(pad) {
			return nil, fmt.Errorf("wecom: PKCS#7 填充内容非法")
		}
	}
	return data[:len(data)-pad], nil
}
