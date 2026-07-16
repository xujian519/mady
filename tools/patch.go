package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// PatchOperations defines pluggable filesystem operations for the patch tool.
type PatchOperations interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(path string, content []byte) error
	Stat(ctx context.Context, path string) (os.FileInfo, error)
}

// DefaultPatchOperations uses the local filesystem.
type DefaultPatchOperations struct{}

func (d DefaultPatchOperations) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (d DefaultPatchOperations) WriteFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0600)
}
func (d DefaultPatchOperations) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// PatchToolConfig configures the patch tool.
type PatchToolConfig struct {
	Operations PatchOperations
	// Sandbox enforces the WorkingDir boundary when Enabled.
	Sandbox WorkingDirSandbox
}

func (c *PatchToolConfig) defaults() {
	if c.Operations == nil {
		c.Operations = DefaultPatchOperations{}
	}
}

// PatchToolInput is the JSON arguments for the patch tool.
type PatchToolInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// PatchToolDetails carries patch metadata.
type PatchToolDetails struct {
	Diff             string `json:"diff"`
	FirstChangedLine *int   `json:"first_changed_line,omitempty"`
}

// NewPatchTool creates a patch tool that replaces old_string with new_string.
func NewPatchTool(cwd string, cfg *PatchToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &PatchToolConfig{}
	}
	cfg.defaults()

	return &agentcore.Tool{
		Name: "patch",
		Description: "通过替换精确字符串来对文件打补丁。" +
			"old_string 必须在文件中恰好匹配一次。" +
			"如果找不到 old_string，工具将返回带建议的错误信息。" +
			"如需多次替换，请使用 edit 替代。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "要修补的文件路径（相对或绝对路径）",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "要替换的精确文本。必须在文件中恰好匹配一次。",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "替换后的新文本",
				},
			},
			"required": []any{"path", "old_string", "new_string"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input PatchToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return resultErrf("invalid arguments: %w", err)
			}

			if input.Path == "" {
				return resultErrf("path is required")
			}
			if input.OldString == "" {
				return resultErrf("old_string is required")
			}

			resolved, err := resolvePathSandboxed(input.Path, cwd, cfg.Sandbox)
			if err != nil {
				return resultErrf("%v", err)
			}
			// When sandbox is enabled, pin the resolved inode to detect
			// symlink swaps between validation and the actual operation.
			if cfg.Sandbox.Enabled {
				if err := pinPath(resolved); err != nil {
					return resultErrf("%v", err)
				}
			}

			// Check file exists.
			if _, err := cfg.Operations.Stat(ctx, resolved); err != nil {
				return resultErrf("file not found: %s", input.Path)
			}

			// Read file.
			data, err := cfg.Operations.ReadFile(ctx, resolved)
			if err != nil {
				return resultErrf("failed to read file: %w", err)
			}

			// Strip BOM if present.
			bom, content := stripBOM(string(data))
			originalEnding := detectLineEnding(content)
			normalized := normalizeToLF(content)

			// Normalize old_string to LF for matching.
			oldNormalized := normalizeToLF(input.OldString)
			newNormalized := normalizeToLF(input.NewString)

			// Check if old_string exists.
			count := strings.Count(normalized, oldNormalized)
			if count == 0 {
				// Try to find closest match for suggestion.
				suggestion := findClosestMatch(normalized, oldNormalized)
				return resultErrf("old_string not found in %s. %s", input.Path, suggestion)
			}
			if count > 1 {
				return resultErrf("old_string found %d times in %s. It must match exactly once. Use edit for multiple replacements.", count, input.Path)
			}

			// Apply replacement.
			newContent := strings.Replace(normalized, oldNormalized, newNormalized, 1)

			if normalized == newContent {
				return resultErrf("no changes made to %s", input.Path)
			}

			// Restore line endings.
			finalContent := bom + restoreLineEndings(newContent, originalEnding)

			// Write back.
			if err := cfg.Operations.WriteFile(resolved, []byte(finalContent)); err != nil {
				return resultErrf("failed to write file: %w", err)
			}

			// Generate diff.
			diff, firstLine := generateDiff(normalized, newContent)

			return result(
				fmt.Sprintf("Successfully patched %s.", input.Path),
				PatchToolDetails{Diff: diff, FirstChangedLine: firstLine},
			)
		},
	}
}

// findClosestMatch tries to find the closest line match for a failed patch.
func findClosestMatch(content, target string) string {
	contentLines := strings.Split(content, "\n")
	targetLines := strings.Split(target, "\n")

	if len(targetLines) == 0 {
		return ""
	}

	// Try to find the first line of target in content.
	firstLine := strings.TrimSpace(targetLines[0])
	if firstLine == "" {
		return ""
	}

	var matches []string
	for i, line := range contentLines {
		if strings.Contains(line, firstLine) {
			contextStart := maxInt(0, i-1)
			contextEnd := minInt(len(contentLines), i+len(targetLines)+1)
			context := strings.Join(contentLines[contextStart:contextEnd], "\n")
			matches = append(matches, fmt.Sprintf("Did you mean around line %d?\n```\n%s\n```", i+1, context))
			if len(matches) >= 3 {
				break
			}
		}
	}

	if len(matches) > 0 {
		return strings.Join(matches, "\n")
	}

	return "Try reading the file first to verify the exact text."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
