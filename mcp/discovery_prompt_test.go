package mcp

// M69 补测：mcp 的 Prompts 发现面与 DiscoveryExtension 生命周期此前为 0% 覆盖。
// 这些是纯粹的「协议载荷 → agentcore.Message」转换，不依赖网络与子进程，
// 适合用表驱动锁定分支语义（text/image/resource/默认兜底）。

import (
	"context"
	"slices"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestDataURL(t *testing.T) {
	cases := []struct {
		name     string
		mimeType string
		data     string
		want     string
	}{
		{"空数据返回空串", "image/png", "", ""},
		{"缺省 MIME 回退 octet-stream", "", "QUJD", "data:application/octet-stream;base64,QUJD"},
		{"显式 MIME", "image/png", "QUJD", "data:image/png;base64,QUJD"},
	}
	for _, c := range cases {
		if got := dataURL(c.mimeType, c.data); got != c.want {
			t.Errorf("%s：dataURL = %q，期望 %q", c.name, got, c.want)
		}
	}
}

func TestPromptMessageAgentMessage(t *testing.T) {
	cases := []struct {
		name          string
		msg           PromptMessage
		wantRole      agentcore.Role
		wantContent   string
		wantImageURLs []string
	}{
		{
			name:        "assistant 文本",
			msg:         PromptMessage{Role: "assistant", Content: PromptContent{Type: "text", Text: "草稿"}},
			wantRole:    agentcore.RoleAssistant,
			wantContent: "草稿",
		},
		{
			name:        "未知角色回落 user",
			msg:         PromptMessage{Role: "system", Content: PromptContent{Type: "text", Text: "hi"}},
			wantRole:    agentcore.RoleUser,
			wantContent: "hi",
		},
		{
			name:          "image 转 data URL 块",
			msg:           PromptMessage{Role: "user", Content: PromptContent{Type: "image", MIMEType: "image/png", Data: "QUJD"}},
			wantRole:      agentcore.RoleUser,
			wantImageURLs: []string{"data:image/png;base64,QUJD"},
		},
		{
			name:        "resource 取内嵌文本",
			msg:         PromptMessage{Role: "user", Content: PromptContent{Type: "resource", Resource: &EmbeddedResource{URI: "file:///a.txt", Text: "正文"}}},
			wantRole:    agentcore.RoleUser,
			wantContent: "正文",
		},
		{
			name:        "resource 无文本时回落 URI",
			msg:         PromptMessage{Role: "user", Content: PromptContent{Type: "resource", Resource: &EmbeddedResource{URI: "file:///a.txt"}}},
			wantRole:    agentcore.RoleUser,
			wantContent: "file:///a.txt",
		},
		{
			name:        "resource 指针为空时内容为空",
			msg:         PromptMessage{Role: "user", Content: PromptContent{Type: "resource"}},
			wantRole:    agentcore.RoleUser,
			wantContent: "",
		},
		{
			name:        "未知类型但带 text 时兜底取 text",
			msg:         PromptMessage{Role: "user", Content: PromptContent{Type: "audio", Text: "转写"}},
			wantRole:    agentcore.RoleUser,
			wantContent: "转写",
		},
		{
			name:        "未知类型无 text 时取 resource 文本",
			msg:         PromptMessage{Role: "user", Content: PromptContent{Type: "audio", Resource: &EmbeddedResource{Text: "内嵌"}}},
			wantRole:    agentcore.RoleUser,
			wantContent: "内嵌",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.msg.AgentMessage()
			if got.Role != c.wantRole {
				t.Errorf("Role = %q，期望 %q", got.Role, c.wantRole)
			}
			if got.Content != c.wantContent {
				t.Errorf("Content = %q，期望 %q", got.Content, c.wantContent)
			}
			var urls []string
			for _, block := range got.Blocks {
				if block.Kind == agentcore.BlockKindImage {
					urls = append(urls, block.URL)
				}
			}
			if !slices.Equal(urls, c.wantImageURLs) {
				t.Errorf("图片块 = %v，期望 %v", urls, c.wantImageURLs)
			}
		})
	}
}

func TestPromptResultAgentMessages(t *testing.T) {
	result := PromptResult{Messages: []PromptMessage{
		{Role: "assistant", Content: PromptContent{Type: "text", Text: "问"}},
		{Role: "user", Content: PromptContent{Type: "text", Text: "答"}},
	}}
	msgs := result.AgentMessages()
	if len(msgs) != 2 {
		t.Fatalf("消息数量 = %d，期望 2", len(msgs))
	}
	if msgs[0].Content != "问" || msgs[1].Content != "答" {
		t.Fatalf("消息内容顺序错位：%#v", msgs)
	}
	if got := (PromptResult{}).AgentMessages(); len(got) != 0 {
		t.Fatalf("空结果应返回空切片，实际 %#v", got)
	}
}

func TestDiscoveryExtensionLifecycle(t *testing.T) {
	t.Run("client 为空时拒绝构造", func(t *testing.T) {
		if _, err := NewDiscoveryExtension(nil, DiscoveryToolConfig{IncludeResources: true}); err == nil {
			t.Fatal("client 为 nil 时应返回错误")
		}
	})

	t.Run("未启用任何工具时拒绝构造", func(t *testing.T) {
		if _, err := NewDiscoveryExtension(&HTTPClient{}, DiscoveryToolConfig{}); err == nil {
			t.Fatal("未启用 resources/prompts 时应返回错误")
		}
	})

	t.Run("默认名称与生命周期", func(t *testing.T) {
		ext, err := NewDiscoveryExtension(&HTTPClient{}, DiscoveryToolConfig{IncludeResources: true, IncludePrompts: true})
		if err != nil {
			t.Fatal(err)
		}
		if ext.Name() != "mcp-discovery" {
			t.Fatalf("默认名称 = %q，期望 mcp-discovery", ext.Name())
		}
		if len(ext.Tools()) == 0 {
			t.Fatal("启用发现工具后 Tools 不应为空")
		}
		if err := ext.Init(context.Background(), nil); err != nil {
			t.Fatalf("Init 应无副作用：%v", err)
		}
		if err := ext.Dispose(); err != nil {
			t.Fatalf("Dispose 应无副作用：%v", err)
		}
	})

	t.Run("自定义名称生效", func(t *testing.T) {
		ext, err := NewDiscoveryExtension(&HTTPClient{}, DiscoveryToolConfig{Name: "custom-discovery", IncludePrompts: true})
		if err != nil {
			t.Fatal(err)
		}
		if ext.Name() != "custom-discovery" {
			t.Fatalf("名称 = %q，期望 custom-discovery", ext.Name())
		}
	})
}
