package mcp

import (
	"crypto/rand"
	"encoding/hex"
)

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
