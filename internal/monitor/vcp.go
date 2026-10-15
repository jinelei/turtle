package monitor

// VCP 特征码常量（VESA Monitor Control Command Set 常用子集）。
const (
	VCPInputSelect uint8 = 0x60
	VCPCapabilities uint8 = 0xF3
	VCPBrightness   uint8 = 0x10
	VCPContrast     uint8 = 0x12
	VCPVolume       uint8 = 0x62
	VCPPowerMode    uint8 = 0xD6
)

// FeatureName 返回常用 VCP 码的中文名，未知码返回 "VCP 0xNN"。
func FeatureName(code uint8) string {
	if n, ok := featureNames[code]; ok {
		return n
	}
	return "VCP " + hexByte(code)
}

var featureNames = map[uint8]string{
	0x02: "新控制值",
	0x04: "复位出厂亮度对比度",
	0x05: "复位出厂几何",
	0x08: "恢复出厂颜色",
	0x0B: "颜色温度",
	0x0C: "色温增量",
	0x0E: "水平图像位置",
	0x10: "亮度",
	0x12: "对比度",
	0x14: "几何梯形校正",
	0x16: "几何枕形校正",
	0x1E: "水平会聚",
	0x20: "垂直会聚",
	0x30: "垂直位置",
	0x3E: "垂直频率",
	0x40: "水平相位",
	0x52: "活动控制",
	0x59: "6轴色温红",
	0x5A: "6轴色温绿",
	0x5B: "6轴色温蓝",
	0x60: "输入源选择",
	0x62: "音频扬声器音量",
	0x63: "扬声器平衡",
	0x6C: "视频黑色级别红",
	0x6E: "视频黑色级别绿",
	0x70: "视频黑色级别蓝",
	0x87: "USB 控制",
	0x8D: "USB-C 状态",
	0xAA: "OSD 语言",
	0xAC: "OSD 水平位置",
	0xAE: "OSD 垂直位置",
	0xB6: "显示控制器类型",
	0xCA: "OSD 透明度",
	0xCC: "OSD 超时",
	0xD6: "显示电源模式",
	0xD9: "DPMS 开关",
	0xDA: "输入源名称",
	0xDC: "显示处理模式",
	0xDF: "垂直频率（详细）",
	0xE0: "水平频率（详细）",
	0xE1: "外接盒型号",
	0xF3: "能力清单（Capabilities）",
}

// StandardInputNames 是 MCCS 标准输入源值（VCP 0x60）到可读名称的映射。
// 参考 VESA VCP 规范与主流显示器实践（Lenovo/DELL/HP 通用值）。
var StandardInputNames = map[uint8]string{
	0x01: "VGA 1",
	0x02: "VGA 2",
	0x03: "DVI 1",
	0x04: "DVI 2",
	0x05: "复合视频 1",
	0x06: "复合视频 2",
	0x07: "S-Video 1",
	0x08: "S-Video 2",
	0x09: "调谐器 1",
	0x0A: "调谐器 2",
	0x0B: "分量视频 1",
	0x0C: "分量视频 2",
	0x0D: "DP 1",
	0x0E: "DP 2",
	0x0F: "DisplayPort 1", // Lenovo 等厂商常用：0x0F = DP1
	0x10: "DisplayPort 2",
	0x11: "HDMI 1",
	0x12: "HDMI 2",
}

// vendorInputNames 是标准表之外的厂商自定义输入值
//（0x31 在 MCCS 中属保留区，Lenovo 用于 USB-C）。
// 实测来源：Lenovo P27h-20，ControlMyMonitor 读出
// 0x60 Possible Values = 15(0x0F DP), 17(0x11 HDMI), 49(0x31 USB-C)。
var vendorInputNames = map[string]map[uint8]string{
	"Lenovo": {
		0x31: "USB-C",
	},
}

// InputNameForVendor 先查厂商扩展映射，再回退标准表。
func InputNameForVendor(vendor string, code uint8) string {
	if m, ok := vendorInputNames[vendor]; ok {
		if n, ok := m[code]; ok {
			return n
		}
	}
	return InputName(code)
}

// InputName 返回输入源值的可读名称，未知值返回带原始值的占位文案。
// P27h-20 等机型的实际值在首次抓包后可由厂商映射扩展。
func InputName(code uint8) string {
	if n, ok := StandardInputNames[code]; ok {
		return n
	}
	return "未知输入源 (" + hexByte(code) + ")"
}

func hexByte(b uint8) string {
	const h = "0123456789ABCDEF"
	return "0x" + string([]byte{h[b>>4], h[b&0x0F]})
}
