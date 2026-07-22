package evidence

import "testing"

func TestPlatformCredibility_Gov(t *testing.T) {
	level := PlatformCredibility("https://www.cnipa.gov.cn/doc")
	if level != CredHigh {
		t.Errorf("gov.cn = %s, want high", level)
	}
}

func TestPlatformCredibility_WeChat(t *testing.T) {
	level := PlatformCredibility("https://mp.weixin.qq.com/s/article")
	if level != CredMediumHigh {
		t.Errorf("wechat = %s, want medium_high", level)
	}
}

func TestPlatformCredibility_B2B(t *testing.T) {
	level := PlatformCredibility("https://www.made-in-china.com/product")
	if level != CredLow {
		t.Errorf("b2b = %s, want low", level)
	}
}
