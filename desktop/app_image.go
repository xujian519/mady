//go:build darwin

package main

// app_image.go — 粘贴图片保存（阶段 4 / Reasonix 对齐：Composer 粘贴图片）。
//
// 前端 Composer 检测到剪贴板图片时，以 data URL（data:image/png;base64,...）调用
// SavePastedImage，本方法解码后保存到项目根 attachments/ 目录，返回相对路径供
// 消息 Markdown 引用（![名称](attachments/pasted-xxx.png)）。

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxPastedImageSize 是粘贴图片的单张上限（10MB）。
const maxPastedImageSize = 10 << 20

// decodePastedImage 解析并解码粘贴图片的 data URL。
// 返回解码后的字节与扩展名；仅接受 data:image/*;base64 形式。
func decodePastedImage(dataURL string) ([]byte, string, error) {
	if !strings.HasPrefix(dataURL, "data:image/") {
		return nil, "", fmt.Errorf("仅支持 data:image/* data URL")
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("data URL 缺少 base64 载荷")
	}
	meta := dataURL[:comma]
	payload := dataURL[comma+1:]

	// 从 MIME 推导扩展名（白名单）
	mime := strings.TrimPrefix(strings.SplitN(meta, ";", 2)[0], "data:")
	ext, ok := map[string]string{
		"image/png":  "png",
		"image/jpeg": "jpg",
		"image/gif":  "gif",
		"image/webp": "webp",
	}[mime]
	if !ok {
		return nil, "", fmt.Errorf("不支持的图片类型 %q", mime)
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("base64 解码失败: %w", err)
	}
	if len(decoded) == 0 {
		return nil, "", fmt.Errorf("图片内容为空")
	}
	if len(decoded) > maxPastedImageSize {
		return nil, "", fmt.Errorf("图片超过 %d MB 上限", maxPastedImageSize>>20)
	}
	return decoded, ext, nil
}

// SavePastedImage 将剪贴板粘贴的图片（data URL）保存到项目根 attachments/ 目录。
// 返回相对项目根的路径（如 attachments/pasted-<ts>.png），供消息 Markdown 引用。
func (a *App) SavePastedImage(dataURL string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	decoded, ext, err := decodePastedImage(dataURL)
	if err != nil {
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}
	attachmentsDir := filepath.Join(cwd, "attachments")
	if err := os.MkdirAll(attachmentsDir, 0750); err != nil {
		return "", fmt.Errorf("SavePastedImage: 创建 attachments 失败: %w", err)
	}

	name := fmt.Sprintf("pasted-%d.%s", time.Now().UnixNano(), ext)
	abs := filepath.Join(attachmentsDir, name)
	// attachments 目录位于项目根内，路径天然受沙箱约束（防御性再校验一次）。
	if !isPathWithinSandbox(abs, cwd) {
		return "", fmt.Errorf("SavePastedImage: 路径逃逸")
	}
	// 原子写（tmp+rename，与 WriteFile 的既有模式一致，B-6）：崩溃不残留半张图。
	tmp, err := os.CreateTemp(attachmentsDir, ".mady-img-*")
	if err != nil {
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(decoded); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return "", fmt.Errorf("SavePastedImage: %w", err)
	}
	rel := filepath.ToSlash(filepath.Join("attachments", name))
	log.Printf("[mady-desktop] saved pasted image: %s (%d bytes)", rel, len(decoded))
	return rel, nil
}
