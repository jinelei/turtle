// Package edid 解析 EDID 1.3/1.4 二进制块中的身份信息：
// 厂商 PnP ID、产品码、型号名描述符与序列号描述符。
// Windows（注册表）与 macOS（IORegistry）取得的 EDID 字节
// 共用本解析器。
package edid

import (
	"errors"
	"strings"
)

// Info 为解析得到的显示器身份信息；缺失字段为空串，
// 由上层统一展示为 "未知"。
type Info struct {
	VendorID    string // 三字母 PnP ID，如 "LEN"
	ProductCode uint16 // 厂商产品码
	Name        string // 描述符中的型号名，如 "P27h-20"
	Serial      string // 描述符中的序列号字符串
}

// 常见显示器厂商 PnP ID → 显示名（查不到时上层直接展示 PnP ID）。
var vendorNames = map[string]string{
	"LEN": "Lenovo",
	"DEL": "Dell",
	"SAM": "Samsung",
	"GSM": "LG",
	"ACI": "ASUS",
	"AOC": "AOC",
	"MSI": "MSI",
	"BNQ": "BenQ",
	"ACR": "Acer",
	"VSC": "ViewSonic",
	"PHL": "Philips",
	"EIZ": "EIZO",
	"APP": "Apple",
	"HPQ": "HP",
	"HWP": "HP",
}

// VendorName 返回厂商显示名。
func (i *Info) VendorName() string {
	if n, ok := vendorNames[i.VendorID]; ok {
		return n
	}
	return i.VendorID
}

// Parse 解析一块 128 字节（或含扩展块的更长）EDID。
func Parse(b []byte) (*Info, error) {
	if len(b) < 128 {
		return nil, errors.New("edid: too short")
	}
	if b[0] != 0x00 || b[1] != 0xFF || b[2] != 0xFF || b[3] != 0xFF ||
		b[4] != 0xFF || b[5] != 0xFF || b[6] != 0xFF || b[7] != 0x00 {
		return nil, errors.New("edid: bad header signature")
	}

	info := &Info{
		VendorID:    vendorID(b[8], b[9]),
		ProductCode: uint16(b[10]) | uint16(b[11])<<8, // little-endian
	}

	// 四个 18 字节描述符位于偏移 54..125。
	for off := 54; off < 126; off += 18 {
		d := b[off : off+18]
		// 详细时序描述符的前 5 字节非全零/0xFB；
		// 显示器名称 0xFC，序列号字符串 0xFF。
		if d[0] == 0 && d[1] == 0 && d[2] == 0 && d[3] == 0xFC {
			info.Name = cleanDescriptor(d[5:])
		}
		if d[0] == 0 && d[1] == 0 && d[2] == 0 && d[3] == 0xFF {
			info.Serial = cleanDescriptor(d[5:])
		}
	}

	return info, nil
}

// vendorID 将两个大端 5-bit 字段还原为三字母 PnP ID。
func vendorID(hi, lo byte) string {
	v := uint16(hi)<<8 | uint16(lo)
	c1 := byte('A' + ((v>>10)&0x1F) - 1)
	c2 := byte('A' + ((v>>5)&0x1F) - 1)
	c3 := byte('A' + (v&0x1F) - 1)
	return string([]byte{c1, c2, c3})
}

// cleanDescriptor 把描述符文本裁剪到行终止符 0x0A/0x00，
// 并去除空格与不可见字符。
func cleanDescriptor(b []byte) string {
	s := strings.TrimRight(string(b), " \x00\n\r\t")
	return strings.TrimSpace(s)
}
