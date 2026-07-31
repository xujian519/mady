package audit

import "testing"

func TestDataRetentionConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DataRetentionConfig
		wantErr bool
	}{
		{"默认配置合法", DefaultRetentionConfig(), false},
		{"零值（永久保留）合法", DataRetentionConfig{}, false},
		{"DefaultDays 为负", DataRetentionConfig{DefaultDays: -1}, true},
		{"AuditRetentionDays 为负", DataRetentionConfig{AuditRetentionDays: -7}, true},
		{"正常天数合法", DataRetentionConfig{DefaultDays: 3650, AuditRetentionDays: 365}, false},
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
