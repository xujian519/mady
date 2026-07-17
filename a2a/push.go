package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// PushNotifier handles webhook-based push notification delivery with
// exponential backoff retry and private-IP protection.
type PushNotifier struct {
	client       *http.Client
	maxRetries   int
	backoff      time.Duration
	allowPrivate bool
}

// NewPushNotifier creates a new push notifier.
func NewPushNotifier() *PushNotifier {
	return &PushNotifier{
		client:     &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
		backoff:    500 * time.Millisecond,
	}
}

func (n *PushNotifier) WithAllowPrivate(allow bool) *PushNotifier {
	n.allowPrivate = allow
	return n
}

// Notify sends a task update to the configured webhook with exponential backoff retry.
func (n *PushNotifier) Notify(ctx context.Context, cfg *PushNotificationConfig, task *Task) error {
	if !n.allowPrivate {
		if err := validateWebhookURL(cfg.URL); err != nil {
			return fmt.Errorf("invalid webhook URL: %w", err)
		}
	}

	payload := map[string]any{
		"taskId": task.ID,
		"state":  task.State,
		"task":   task,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal push payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= n.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := n.backoff * time.Duration(1<<uint(attempt-1))
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
			timer.Stop()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create push request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}

		resp, err := n.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send push notification: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("push webhook returned %d: %s", resp.StatusCode, string(respBody))
			if resp.StatusCode >= 500 {
				continue
			}
			return lastErr
		}

		return nil
	}

	return fmt.Errorf("push notification failed after %d retries: %w", n.maxRetries, lastErr)
}

func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("private IP addresses not allowed")
		}
		return nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("lookup %q: %w", host, err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("host %q resolves to private IP %s", host, addr)
		}
	}
	return nil
}

var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16",
		"::1/128", "fc00::/7", "fe80::/10",
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		_, n, _ := net.ParseCIDR(s)
		out = append(out, n)
	}
	return out
}()

func isPrivateIP(ip net.IP) bool {
	for _, network := range privateRanges {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
