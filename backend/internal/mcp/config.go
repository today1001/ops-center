package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// 危险等级定义
// L0 只读查询  L1 主动探测  L2 写入登记  L3 删除/高危（默认禁用）
var LevelRank = map[string]int{"L0": 0, "L1": 1, "L2": 2, "L3": 3}

const DefaultConfigPath = "/home/lt/ops-center/mcp-config.json"

// TokenEntry 访问令牌；MaxLevel 限定该令牌可调用的最高危险等级
type TokenEntry struct {
	Key       string `json:"key"`
	Remark    string `json:"remark,omitempty"`
	MaxLevel  string `json:"max_level,omitempty"` // 空 = L3（不限制）
	CreatedAt string `json:"created_at,omitempty"`
}

// UnmarshalJSON 兼容旧格式（纯字符串 token → 视为 L3 不限制）
func (t *TokenEntry) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		t.Key, t.MaxLevel = s, "L3"
		return nil
	}
	type alias TokenEntry
	var a alias
	if err := json.Unmarshal(b, (*alias)(&a)); err != nil {
		return err
	}
	*t = TokenEntry(a)
	return nil
}

// Config MCP 服务器配置
type Config struct {
	Listen         string       `json:"listen"`
	Tokens         []TokenEntry `json:"tokens"`
	Levels         map[string]bool `json:"levels"`         // L0-L3 各级开关
	ActionsEnabled bool         `json:"actions_enabled"` // 写入类（L2+）总开关
	AuditLog       string       `json:"audit_log"`
}

// Normalize 补默认值
func (c *Config) Normalize() {
	if c.Listen == "" {
		c.Listen = ":7780"
	}
	if c.AuditLog == "" {
		c.AuditLog = "/home/lt/ops-center/mcp-audit.log"
	}
	if c.Levels == nil {
		c.Levels = map[string]bool{}
	}
	for _, lv := range []string{"L0", "L1", "L2", "L3"} {
		if _, ok := c.Levels[lv]; !ok {
			c.Levels[lv] = lv != "L3" // L3 默认禁用
		}
	}
}

// TokenMaxLevel 取令牌的最高可用等级（未匹配 = L3 不限制）
func (c *Config) TokenMaxLevel(key string) string {
	for _, t := range c.Tokens {
		if t.Key == key {
			if t.MaxLevel == "" {
				return "L3"
			}
			return t.MaxLevel
		}
	}
	return "L3"
}

// LoadConfig 加载配置
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	c.Normalize()
	if len(c.Tokens) == 0 {
		return nil, errors.New("tokens 不能为空")
	}
	return &c, nil
}

// SaveConfig 写回配置（0600）
func SaveConfig(path string, c *Config) error {
	c.Normalize()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

// NewToken 生成新令牌
func NewToken(remark, maxLevel string) TokenEntry {
	return TokenEntry{
		Key:       fmt.Sprintf("%x", time.Now().UnixNano()) + randHex(24),
		Remark:    remark,
		MaxLevel:  maxLevel,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
}
