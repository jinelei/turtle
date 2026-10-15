package monitor

import (
	"errors"
	"strings"
)

// Capabilities 是 VCP 0xF3 capabilities 字符串解析结果。
type Capabilities struct {
	Protocol string   // prot(...)
	Type     string   // type(...)
	Model    string   // model(...)
	VCP      []uint8  // vcp(...) 中声明支持的特征码
	Allowed  map[uint8][]uint8 // 表类型特征的可选值，如 0x60: [0x0F,0x11,0x31]
	Raw      string   // 原始字符串
	Partial  bool     // 解析不完整（括号不配对/被截断）
}

// Supported 报告某 VCP 码是否在能力清单中声明；
// 清单缺失（空）时返回 true 以允许尝试读取。
func (c *Capabilities) Supported(code uint8) bool {
	if len(c.VCP) == 0 {
		return true
	}
	for _, v := range c.VCP {
		if v == code {
			return true
		}
	}
	return false
}

// ParseCapabilities 解析形如
// `(prot(monitor)type(LCD)model(P27h-20)cmds(01 02 03 0C F3)vcp(10 12 60 62)...)`
// 的 capabilities 字符串。解析器采用括号配对扫描：
// 括号不配对时输出已解析部分并标记 Partial。
func ParseCapabilities(raw string) (*Capabilities, error) {
	c := &Capabilities{Raw: raw}
	s := strings.TrimSpace(raw)
	if s == "" {
		return c, errors.New("empty capabilities string")
	}
	if strings.Count(s, "(") != strings.Count(s, ")") {
		c.Partial = true
	}

	// 顶层外壳的括号可有可无；直接在整串中找各命名组。
	c.Protocol = firstGroup(s, "prot")
	c.Type = firstGroup(s, "type")
	c.Model = firstGroup(s, "model")

	vcpBody := firstGroup(s, "vcp")
	if vcpBody != "" {
		c.VCP, c.Allowed = parseVCPList(vcpBody)
	}

	if c.Partial {
		return c, errors.New("malformed capabilities string: unbalanced parentheses")
	}
	return c, nil
}

// firstGroup 返回 `name(...)` 的括号内文本（最近一层、不区分大小写）。
func firstGroup(s, name string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, strings.ToLower(name)+"(")
	if idx < 0 {
		return ""
	}
	open := idx + len(name)
	if open >= len(s) || s[open] != '(' {
		return ""
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open+1 : i]
			}
		}
	}
	return s[open+1:] // 截断：返回剩余全部
}

// parseVCPList 解析 vcp 段。真实格式中括号两侧通常没有空格，
// 如 `60(0F 11 31)10 12` 或 `60(0F 11 31)62`：顶层每个十六进制
// token 是一个支持的 VCP 码；码后紧跟括号时，括号内为其可选值。
func parseVCPList(body string) (codes []uint8, allowed map[uint8][]uint8) {
	i, n := 0, len(body)
	for i < n {
		for i < n && !isHexChar(body[i]) {
			i++ // 跳过空格与杂散括号
		}
		start := i
		for i < n && isHexChar(body[i]) {
			i++
		}
		code, ok := parseHexToken(body[start:i])
		if !ok {
			continue
		}
		codes = append(codes, code)

		for i < n && body[i] == ' ' {
			i++
		}
		if i >= n || body[i] != '(' {
			continue
		}
		i++ // 消费 '('
		var vals []uint8
		for i < n {
			for i < n && body[i] == ' ' {
				i++
			}
			if i < n && body[i] == ')' {
				i++
				break
			}
			s := i
			for i < n && isHexChar(body[i]) {
				i++
			}
			if v, ok := parseHexToken(body[s:i]); ok {
				vals = append(vals, v)
			} else if i == s {
				i++ // 防止异常字符卡死
			}
		}
		if len(vals) > 0 {
			if allowed == nil {
				allowed = map[uint8][]uint8{}
			}
			allowed[code] = vals
		}
	}
	return codes, allowed
}

func isHexChar(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// parseHexToken 解析 capabilities 中的十六进制 token：
// 接受 "0x10" 与裸 "10"/"F3"（按十六进制）。
func parseHexToken(tok string) (uint8, bool) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return 0, false
	}
	tok = strings.TrimPrefix(tok, "0x")
	tok = strings.TrimPrefix(tok, "0X")
	var v uint
	for _, r := range tok {
		var d uint
		switch {
		case r >= '0' && r <= '9':
			d = uint(r - '0')
		case r >= 'a' && r <= 'f':
			d = uint(r-'a') + 10
		case r >= 'A' && r <= 'F':
			d = uint(r-'A') + 10
		default:
			return 0, false
		}
		v = v*16 + d
		if v > 0xFF {
			return 0, false
		}
	}
	return uint8(v), true
}
