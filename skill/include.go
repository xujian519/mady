package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxIncludeDepth is the maximum nesting level for recursive include expansion.
// A value of 3 means A can include B, B can include C, but C cannot include D.
const maxIncludeDepth = 3

// includeTagPattern matches <include ref="path/to/file.md" /> with optional
// whitespace flexibility (spaces before/after tag name, around =, etc.).
// The ref value is captured in group 1. Only relative paths (no "..") are
// accepted to prevent path traversal.
var includeTagPattern = regexp.MustCompile(
	`<include\s+ref\s*=\s*"([^"]*)"\s*/>`,
)

// ExpandIncludes recursively expands <include ref="..."/> tags in the given
// markdown body. The baseDir is the directory of the SKILL.md file being
// processed, against which relative paths are resolved.
//
// Safety guarantees:
//   - Maximum recursion depth of maxIncludeDepth (3 layers)
//   - Paths containing ".." are rejected to prevent directory traversal
//   - Cyclic includes are detected via a visited-path stack
//   - Missing files return a clear error message
//   - Each file is expanded at most once per top-level invocation (cached)
func ExpandIncludes(baseDir string, body string) (string, error) {
	cache := make(map[string]string)
	return expandIncludesRecursive(baseDir, body, 0, []string{}, cache)
}

// expandIncludesRecursive is the internal recursive implementation.
func expandIncludesRecursive(
	baseDir string,
	body string,
	depth int,
	visited []string,
	cache map[string]string,
) (string, error) {
	if depth >= maxIncludeDepth {
		return "", fmt.Errorf("include depth exceeded maximum of %d layers", maxIncludeDepth)
	}

	var result strings.Builder
	lastEnd := 0

	matches := includeTagPattern.FindAllStringSubmatchIndex(body, -1)
	for _, match := range matches {
		// Append text before this tag
		result.WriteString(body[lastEnd:match[0]])

		// Extract the ref path (group 1 is at indices 2,3 in the submatch)
		refPath := body[match[2]:match[3]]
		refPath = strings.TrimSpace(refPath)

		// Validate path: reject empty and path-traversal attempts
		if refPath == "" {
			return "", fmt.Errorf("empty ref in <include> tag")
		}
		cleanPath := filepath.Clean(refPath)
		if strings.Contains(cleanPath, "..") {
			return "", fmt.Errorf(
				"include path %q is not allowed: path traversal detected (contains '..')",
				refPath,
			)
		}
		if filepath.IsAbs(cleanPath) {
			return "", fmt.Errorf(
				"include path %q is not allowed: absolute paths are not supported",
				refPath,
			)
		}

		// Resolve the full path relative to baseDir
		fullPath := filepath.Join(baseDir, cleanPath)

		// Check for cycles
		for _, v := range visited {
			if v == fullPath {
				return "", fmt.Errorf(
					"circular include detected: %s is already included in this chain",
					fullPath,
				)
			}
		}

		// Use cached content if available
		var content string
		if cached, ok := cache[fullPath]; ok {
			content = cached
		} else {
			data, err := os.ReadFile(fullPath) //nolint:gosec // G304: path from include resolution with known root
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf(
						"included file not found: %s (referenced from base directory %s)",
						cleanPath, baseDir,
					)
				}
				return "", fmt.Errorf("failed to read included file %s: %w", fullPath, err)
			}
			content = string(data)
			cache[fullPath] = content
		}

		// If the included file has SKILL.md-style frontmatter, strip it
		// so only the body content is included. We reuse the existing
		// parseFrontmatter function.
		if strings.HasPrefix(content, "---\n") {
			_, bodyContent, _ := parseFrontmatter(content, fullPath)
			if strings.TrimSpace(bodyContent) != "" {
				content = bodyContent
			}
		}

		// Recursively expand includes in the included content.
		// The baseDir for nested includes is the directory of the included file.
		nestedDir := filepath.Dir(fullPath)
		expanded, err := expandIncludesRecursive(
			nestedDir,
			content,
			depth+1,
			append(visited, fullPath),
			cache,
		)
		if err != nil {
			return "", fmt.Errorf("in %s: %w", cleanPath, err)
		}

		result.WriteString(strings.TrimSpace(expanded))
		result.WriteString("\n")

		lastEnd = match[1]
	}

	// Append remaining text after the last tag
	result.WriteString(body[lastEnd:])

	return result.String(), nil
}
