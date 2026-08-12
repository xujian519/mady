package tools

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

func (s *CDPSupervisor) InjectDialogBridge() error {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		return fmt.Errorf("supervisor not connected")
	}

	bridgeScript := `
(function() {
	const originalAlert = window.alert;
	const originalConfirm = window.confirm;
	const originalPrompt = window.prompt;

	window.alert = function(msg) {
		const xhr = new XMLHttpRequest();
		xhr.open('POST', 'http://__dialog_bridge__/alert', false);
		xhr.setRequestHeader('Content-Type', 'application/json');
		xhr.send(JSON.stringify({message: msg}));
		const resp = JSON.parse(xhr.responseText);
		return resp.accepted;
	};

	window.confirm = function(msg) {
		const xhr = new XMLHttpRequest();
		xhr.open('POST', 'http://__dialog_bridge__/confirm', false);
		xhr.setRequestHeader('Content-Type', 'application/json');
		xhr.send(JSON.stringify({message: msg}));
		const resp = JSON.parse(xhr.responseText);
		return resp.accepted;
	};

	window.prompt = function(msg, def) {
		const xhr = new XMLHttpRequest();
		xhr.open('POST', 'http://__dialog_bridge__/prompt', false);
		xhr.setRequestHeader('Content-Type', 'application/json');
		xhr.send(JSON.stringify({message: msg, default: def}));
		const resp = JSON.parse(xhr.responseText);
		return resp.value;
	};
})();
`

	return chromedp.Run(ctx, chromedp.Evaluate(bridgeScript, nil))
}

type DialogBridgeHandler struct {
	mu       sync.Mutex
	dialogs  []map[string]string
	policy   DialogPolicy
	timeout  time.Duration
	deadline time.Time
}

func NewDialogBridgeHandler(policy DialogPolicy, timeout time.Duration) *DialogBridgeHandler {
	return &DialogBridgeHandler{
		policy:   policy,
		timeout:  timeout,
		deadline: time.Now().Add(timeout),
	}
}

func (h *DialogBridgeHandler) HandleDialog(dialogType string, message string, defaultValue string) map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if time.Now().After(h.deadline) {
		return map[string]string{"accepted": "false", "value": ""}
	}

	h.dialogs = append(h.dialogs, map[string]string{
		"type":    dialogType,
		"message": message,
		"default": defaultValue,
	})

	switch h.policy {
	case DialogAutoDismiss:
		return map[string]string{"accepted": "false", "value": ""}
	case DialogAutoAccept:
		return map[string]string{"accepted": "true", "value": defaultValue}
	default:
		return map[string]string{"accepted": "pending", "value": ""}
	}
}

func (h *DialogBridgeHandler) GetDialogs() []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]map[string]string, len(h.dialogs))
	copy(result, h.dialogs)
	return result
}

func (h *DialogBridgeHandler) RespondToPending(accepted bool, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.dialogs) > 0 {
		last := h.dialogs[len(h.dialogs)-1]
		last["accepted"] = fmt.Sprintf("%t", accepted)
		last["value"] = value
	}
}

func formatDialogs(dialogs map[string]*DialogInfo) string {
	if len(dialogs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Pending Dialogs\n")
	for id, d := range dialogs {
		fmt.Fprintf(&sb, "- [%s] %s: \"%s\"", id, d.Type, d.Message)
		if d.Default != "" {
			fmt.Fprintf(&sb, " (default: %s)", d.Default)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatFrameTree(frames []*FrameInfo, truncated bool) string {
	if len(frames) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Frame Tree\n")
	for _, f := range frames {
		fmt.Fprintf(&sb, "- Frame %s: %s", f.ID, f.URL)
		if f.Name != "" {
			fmt.Fprintf(&sb, " (%s)", f.Name)
		}
		sb.WriteString("\n")
	}
	if truncated {
		sb.WriteString("[... frame tree truncated]\n")
	}
	return sb.String()
}
