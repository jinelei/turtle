//go:build windows

package ddc

import "testing"

func TestParseDisplayInterfacePath(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		wantMfg  string
		wantInst string
		wantOK   bool
	}{
		{
			name:     "小米电视物理监视器接口路径",
			path:     `\\?\DISPLAY#XMD0076#5&3016bf3c&0&UID4356#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`,
			wantMfg:  "XMD0076",
			wantInst: "5&3016bf3c&0&UID4356",
			wantOK:   true,
		},
		{
			name:     "P27h-20 接口路径",
			path:     `\\?\DISPLAY#LEN61E9#5&3016bf3c&0&UID4354#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`,
			wantMfg:  "LEN61E9",
			wantInst: "5&3016bf3c&0&UID4354",
			wantOK:   true,
		},
		{
			name:   "旧式 MONITOR 接口名不可用于定位注册表",
			path:   `MONITOR\XMD0076\{4d36e96e-e325-11ce-bfc1-08002be10318}\0008`,
			wantOK: false,
		},
		{
			name:   "空串",
			path:   "",
			wantOK: false,
		},
		{
			name:   "缺少实例段",
			path:   `\\?\DISPLAY#XMD0076#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`,
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mfg, inst, ok := parseDisplayInterfacePath(tc.path)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (mfg != tc.wantMfg || inst != tc.wantInst) {
				t.Fatalf("got (%q, %q), want (%q, %q)", mfg, inst, tc.wantMfg, tc.wantInst)
			}
		})
	}
}
