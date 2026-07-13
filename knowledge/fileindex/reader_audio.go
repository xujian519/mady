package fileindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readAudio returns metadata for audio files. ASR is not performed here;
// that would require Whisper or a speech-to-text service (M3+).
//
// The function attempts to estimate the duration from file metadata,
// and returns a cost notice explaining how long ASR would take.
func (fr *FileReader) readAudio(path string, info os.FileInfo) *FileReadResult {
	ext := strings.ToLower(filepath.Ext(path))

	// Rough duration estimate based on file size at 128kbps MP3 (~16KB/s).
	// For M4A/OGG at similar bitrates the estimate is a rough proxy.
	estimatedSeconds := int(info.Size() / (16 * 1024)) // 16KB per second at 128kbps
	if estimatedSeconds < 1 {
		estimatedSeconds = 1
	}

	estimatedMinutes := estimatedSeconds / 60
	asrTimeEstimate := estimatedMinutes / 2 // ASR roughly 2x faster than real-time
	if asrTimeEstimate < 1 {
		asrTimeEstimate = 1
	}

	durationStr := fmt.Sprintf("%d 分 %d 秒", estimatedMinutes, estimatedSeconds%60)
	if estimatedMinutes == 0 {
		durationStr = fmt.Sprintf("%d 秒", estimatedSeconds)
	}

	meta := map[string]string{
		"type":               ext,
		"size_bytes":         fmt.Sprintf("%d", info.Size()),
		"estimated_duration": durationStr,
		"estimated_seconds":  fmt.Sprintf("%d", estimatedSeconds),
		"duration_source":    "estimated_from_size",
	}

	costNotice := fmt.Sprintf(
		"音频文件（%s），预估时长 %s。语音转写约需 %d 分钟。当前版本不支持自动转写，建议人工听取。",
		ext, durationStr, asrTimeEstimate,
	)

	return &FileReadResult{
		Content:    "",
		Confidence: 0.0,
		Metadata:   meta,
		CostNotice: costNotice,
	}
}
