package tools

import (
	"testing"
	"time"
)

func TestBrowserToolConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BrowserToolConfig
		wantErr bool
	}{
		{"零值合法（defaults 填充）", BrowserToolConfig{}, false},
		{"正常配置合法", BrowserToolConfig{CommandTimeout: 30 * time.Second, ViewportWidth: 1280, ViewportHeight: 720, MaxImageSize: 1048576}, false},
		{"CommandTimeout 为负", BrowserToolConfig{CommandTimeout: -1 * time.Second}, true},
		{"DialogTimeout 为负", BrowserToolConfig{DialogTimeout: -5 * time.Second}, true},
		{"InactivityTimeout 为负", BrowserToolConfig{InactivityTimeout: -1 * time.Minute}, true},
		{"视口宽度为负", BrowserToolConfig{ViewportWidth: -100}, true},
		{"视口高度为负", BrowserToolConfig{ViewportHeight: -200}, true},
		{"MaxImageSize 为负", BrowserToolConfig{MaxImageSize: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
