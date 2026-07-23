package tools

import (
	"net"
	"testing"
)

func TestIsDisallowedIP(t *testing.T) {
	tests := []struct {
		ip       string
		disallow bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback IPv6
		{"192.168.1.1", true},     // private
		{"10.0.0.1", true},        // private
		{"172.16.0.1", true},      // private
		{"169.254.169.254", true}, // link-local (cloud metadata)
		{"169.254.1.1", true},     // link-local
		{"0.0.0.0", true},         // unspecified
		{"224.0.0.1", true},       // multicast
		{"8.8.8.8", false},        // public DNS
		{"1.1.1.1", false},        // public DNS
		{"93.184.216.34", false},  // example.com
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("invalid test IP: %s", tc.ip)
		}
		got := isDisallowedIP(ip)
		if got != tc.disallow {
			t.Errorf("isDisallowedIP(%s) = %v, want %v", tc.ip, got, tc.disallow)
		}
	}
}

func TestSafeDialControl_BlocksLoopback(t *testing.T) {
	err := safeDialControl("tcp", "127.0.0.1:8080", nil)
	if err == nil {
		t.Error("safeDialControl should block 127.0.0.1")
	}
}

func TestSafeDialControl_BlocksCloudMetadata(t *testing.T) {
	err := safeDialControl("tcp", "169.254.169.254:80", nil)
	if err == nil {
		t.Error("safeDialControl should block 169.254.169.254")
	}
}

func TestSafeDialControl_BlocksPrivate(t *testing.T) {
	err := safeDialControl("tcp", "192.168.1.1:443", nil)
	if err == nil {
		t.Error("safeDialControl should block 192.168.x.x")
	}
}

func TestSafeDialControl_AllowsPublic(t *testing.T) {
	err := safeDialControl("tcp", "8.8.8.8:53", nil)
	if err != nil {
		t.Errorf("safeDialControl should allow public IP 8.8.8.8, got: %v", err)
	}
}

func TestNewSSRFSafeHTTPClient(t *testing.T) {
	client := newSSRFSafeHTTPClient(10)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 10 {
		t.Errorf("timeout = %v", client.Timeout)
	}
	if client.Transport == nil {
		t.Error("expected non-nil Transport with SSRF guard")
	}
}

func TestNewSSRFSafeTransport_DialerConfigured(t *testing.T) {
	tr := newSSRFSafeTransport()
	if tr.DialContext == nil {
		t.Error("expected DialContext on Transport")
	}
}
