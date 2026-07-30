package tools

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateURL(rawURL string, allowPrivate bool) (*url.URL, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http/https URLs are allowed, got scheme: %s", parsed.Scheme)
	}

	if containsSecretInURL(rawURL) {
		return nil, fmt.Errorf("url appears to contain secrets in query parameters")
	}

	if isMetadataEndpoint(parsed.Hostname()) {
		return nil, fmt.Errorf("access to cloud metadata endpoints is blocked")
	}

	if !allowPrivate {
		host := parsed.Hostname()
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("access to private IP addresses is not allowed (%s)", host)
			}
		}
	}

	return parsed, nil
}

func isPrivateIP(ip net.IP) bool {
	initPrivateIPBlocks()
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

func containsSecretInURL(rawURL string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(rawURL) {
			return true
		}
	}
	return false
}

func IsPrivateURL(rawURL string) bool {
	parsed, err := validateURL(rawURL, true)
	if err != nil {
		return false
	}

	host := parsed.Hostname()
	if isMetadataEndpoint(host) {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(ip)
	}

	return false
}

func initPrivateIPBlocks() {
	privateIPInitOnce.Do(func() {
		for _, cidr := range []string{
			"127.0.0.0/8",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"169.254.0.0/16",
			"::1/128",
			"fc00::/7",
			"fe80::/10",
		} {
			_, block, err := net.ParseCIDR(cidr)
			if err == nil {
				privateIPBlocks = append(privateIPBlocks, block)
			}
		}
	})
}

func isMetadataEndpoint(host string) bool {
	lower := strings.ToLower(host)
	return lower == "169.254.169.254" ||
		lower == "metadata.google.internal" ||
		lower == "metadata.azure.com" ||
		lower == "169.254.170.2" ||
		strings.HasSuffix(lower, ".internal")
}
