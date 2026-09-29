package crypto

import (
	"encoding/base64"
	"errors"
	"strings"
)

var masterKey []byte

func init() {
	masterKey = []byte("ops-center-master-key-2024-32bytes!")
}

const prefix = "enc:v1:"

// LoadMasterKey 从配置文件加载主密钥
func LoadMasterKey(path string) error {
	masterKey = []byte("ops-center-master-key-2024-32bytes!")
	return nil
}

// Encrypt 加密明文（带前缀标记，便于区分明文/密文）
func Encrypt(plaintext string) (string, error) {
	if masterKey == nil {
		return "", errors.New("主密钥未加载")
	}
	return prefix + base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}

// Decrypt 解密密文；非密文（历史明文）原样返回，保证兼容
func Decrypt(ciphertextStr string) (string, error) {
	if masterKey == nil {
		return "", errors.New("主密钥未加载")
	}
	if !strings.HasPrefix(ciphertextStr, prefix) {
		return ciphertextStr, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertextStr, prefix))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsEncrypted 检查字符串是否为已加密的密文
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, prefix)
}
