package ocr

import (
	"os"
	"strings"
)

func mirrorCandidates(url string) []string {
	if os.Getenv("MADY_DISABLE_GH_MIRROR") == "1" {
		return []string{url}
	}

	var out []string
	seen := map[string]bool{}
	add := func(u string) {
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}

	if proxy := strings.TrimRight(os.Getenv("MADY_GH_PROXY"), "/"); proxy != "" {
		add(proxy + "/" + url)
	}

	if isGitHubURL(url) {
		for _, p := range builtinGHProxies {
			add(p + "/" + url)
		}
	}

	if jsd := toJsDelivr(url); jsd != "" {
		add(jsd)
	}

	add(url)
	return out
}

var builtinGHProxies = []string{
	"https://ghfast.top",
	"https://gh-proxy.com",
	"https://ghproxy.net",
}

func isGitHubURL(url string) bool {
	return strings.HasPrefix(url, "https://github.com/") ||
		strings.HasPrefix(url, "https://raw.githubusercontent.com/") ||
		strings.HasPrefix(url, "https://objects.githubusercontent.com/")
}

func toJsDelivr(url string) string {
	const prefix = "https://raw.githubusercontent.com/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	rest := url[len(prefix):]
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) < 4 {
		return ""
	}
	user, repo, ref, path := parts[0], parts[1], parts[2], parts[3]
	return "https://cdn.jsdelivr.net/gh/" + user + "/" + repo + "@" + ref + "/" + path
}
