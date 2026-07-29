package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/xujian519/mady/pkg/ocr"
)

func runOCRCLI(args []string) error {
	if len(args) == 0 {
		printOCRUsage()
		return fmt.Errorf("ocr: missing subcommand")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "recognize", "rec":
		return runOCRRecognize(subArgs)
	case "ensure":
		return runOCREnsure(subArgs)
	case "status":
		return runOCRStatus()
	default:
		printOCRUsage()
		return fmt.Errorf("ocr: unknown subcommand %q", sub)
	}
}

func printOCRUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  mady ocr recognize <path>  对本地图片做 OCR 识别，输出文字
  mady ocr ensure            下载 OCR 模型（~40MB），无需识别
  mady ocr status            查看模型就绪状态

Options (recognize):
  --json            JSON 格式输出（含文本框坐标）
  --progress        显示下载进度

Examples:
  mady ocr recognize scan.png
  mady ocr recognize --json invoice.jpg
  mady ocr recognize --progress scan.png
  mady ocr ensure --progress
  mady ocr status
`)
}

func runOCRRecognize(args []string) error {
	var jsonOutput bool
	var showProgress bool
	var imagePath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--progress":
			showProgress = true
		default:
			imagePath = args[i]
		}
	}

	if imagePath == "" {
		return fmt.Errorf("ocr recognize: 缺少图片路径")
	}

	engine := ocr.Global()
	if !engine.IsReady() {
		if err := engine.EnsureAssets(progressPrinter); err != nil {
			return fmt.Errorf("OCR 模型下载失败: %w", err)
		}
	} else if showProgress {
		fmt.Fprintln(os.Stderr, "OCR 模型已就绪")
	}

	results, err := engine.Recognize(imagePath)
	if err != nil {
		return fmt.Errorf("OCR 识别失败: %w", err)
	}

	if jsonOutput {
		type ocrResult struct {
			Text string `json:"text"`
			Box  [4]int `json:"box"`
		}
		out := make([]ocrResult, len(results))
		for i, r := range results {
			out[i] = ocrResult{Text: r.Text, Box: r.Box}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	var lines []string
	var currentLine []string
	prevYCenter := -1
	for _, r := range results {
		y := (r.Box[1] + r.Box[3]) / 2
		if prevYCenter < 0 || abs(y-prevYCenter) < (r.Box[3]-r.Box[1])/2+1 {
			currentLine = append(currentLine, r.Text)
		} else {
			lines = append(lines, strings.Join(currentLine, " "))
			currentLine = []string{r.Text}
		}
		prevYCenter = y
	}
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, " "))
	}
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

func runOCREnsure(args []string) error {
	showProgress := false
	for _, a := range args {
		if a == "--progress" {
			showProgress = true
			break
		}
	}

	var pfn ocr.ProgressFunc
	if showProgress {
		pfn = progressPrinter
	}
	if err := ocr.Global().EnsureAssets(pfn); err != nil {
		return fmt.Errorf("OCR 模型下载失败: %w", err)
	}
	fmt.Fprintln(os.Stderr, "OCR 模型已就绪:", ocr.Global().CacheDir())
	return nil
}

func runOCRStatus() error {
	engine := ocr.Global()
	fmt.Printf("缓存目录: %s\n", engine.CacheDir())
	if engine.IsReady() {
		fmt.Println("就绪状态: 已就绪")
		return nil
	}
	fmt.Println("就绪状态: 未就绪（运行 mady ocr ensure 下载模型）")
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func progressPrinter(name string, current, total int64) {
	if total <= 0 {
		fmt.Fprintf(os.Stderr, "\r下载 %s ... %d bytes", name, current)
	} else {
		pct := float64(current) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "\r下载 %s ... %.0f%% (%d/%d MB)", name, pct, current/(1<<20), total/(1<<20))
	}
	if current >= total {
		fmt.Fprintln(os.Stderr)
	}
}
