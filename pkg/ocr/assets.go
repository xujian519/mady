package ocr

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	detModelFile = "PP-OCRv5_mobile_det.onnx"
	recModelFile = "PP-OCRv5_mobile_rec.onnx"
	clsModelFile = "ch_ppocr_mobile_v2.0_cls_infer.onnx"
	dictFile     = "ppocrv5_dict.txt"
	readyMarker  = ".ready"

	maxDecompressSize = 1 << 30 // 1GB decompression bomb limit
)

var modelURLs = modelURLSet{
	det:  "https://github.com/MeKo-Christian/paddleocr-onnx/releases/download/v1.0.0/PP-OCRv5_mobile_det.onnx",
	rec:  "https://github.com/MeKo-Christian/paddleocr-onnx/releases/download/v1.0.0/PP-OCRv5_mobile_rec.onnx",
	cls:  "https://github.com/MeKo-Christian/paddleocr-onnx/releases/download/v1.0.0/ch_ppocr_mobile_v2.0_cls_infer.onnx",
	dict: "https://raw.githubusercontent.com/PaddlePaddle/PaddleOCR/main/ppocr/utils/dict/ppocrv5_dict.txt",
}

type modelURLSet struct {
	det, rec, cls, dict string
}

type ProgressFunc func(name string, current, total int64)

func EnsureAssets(cacheDir string, onProgress ProgressFunc) error {
	if isReady(cacheDir) {
		return nil
	}
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return err
	}

	libPath := filepath.Join(cacheDir, ortLibName)
	if !fileExists(libPath) {
		if err := downloadAndExtractORT(cacheDir, onProgress); err != nil {
			return fmt.Errorf("download onnxruntime library: %w", err)
		}
	}
	if err := ensureModel(cacheDir, detModelFile, modelURLs.det, onProgress); err != nil {
		return err
	}
	if err := ensureModel(cacheDir, recModelFile, modelURLs.rec, onProgress); err != nil {
		return err
	}
	if err := ensureModel(cacheDir, clsModelFile, modelURLs.cls, onProgress); err != nil {
		return fmt.Errorf("download cls model: %w", err)
	}

	dictPath := filepath.Join(cacheDir, dictFile)
	if !fileExists(dictPath) {
		if err := downloadFile(mirrorCandidates(modelURLs.dict), dictPath, dictFile, onProgress); err != nil {
			return fmt.Errorf("download dict: %w", err)
		}
	}

	return os.WriteFile(filepath.Join(cacheDir, readyMarker), []byte("ok"), 0600)
}

func ensureModel(cacheDir, filename, url string, onProgress ProgressFunc) error {
	path := filepath.Join(cacheDir, filename)
	if fileExists(path) {
		return nil
	}
	return downloadFile(mirrorCandidates(url), path, filename, onProgress)
}

func isReady(cacheDir string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, readyMarker))
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var downloadClient = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 8 * time.Second,
		TLSHandshakeTimeout:   6 * time.Second,
	},
}

func downloadFile(urls []string, destPath, displayName string, onProgress ProgressFunc) error {
	if len(urls) == 0 {
		return fmt.Errorf("downloadFile: no candidate URLs")
	}
	var lastErr error
	for _, url := range urls {
		err := tryDownload(url, destPath, displayName, onProgress)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("all mirrors failed, last error: %w", lastErr)
}

func tryDownload(url, destPath, displayName string, onProgress ProgressFunc) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	tmpPath := destPath + ".tmp"
	dst, err := os.Create(tmpPath) //nolint:gosec // app-controlled path
	if err != nil {
		return err
	}

	var reader io.Reader = resp.Body
	if onProgress != nil {
		reader = &progressReader{
			reader: resp.Body, total: resp.ContentLength,
			name: displayName, callback: onProgress,
		}
	}

	_, err = io.Copy(dst, reader)
	_ = dst.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, destPath)
}

func downloadAndExtractORT(cacheDir string, onProgress ProgressFunc) error {
	tmp, err := os.CreateTemp("", "ort-*."+ortArchiveFormat)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := downloadFile(mirrorCandidates(ortDownloadURL), tmpPath, ortLibName, onProgress); err != nil {
		return err
	}

	dest := filepath.Join(cacheDir, ortLibName)
	switch ortArchiveFormat {
	case "tgz":
		if err := extractFromTgz(tmpPath, ortArchiveLibPath, dest); err != nil {
			return err
		}
	case "zip":
		if err := extractFromZip(tmpPath, ortArchiveLibPath, dest); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported archive format: %s", ortArchiveFormat)
	}
	return os.Chmod(dest, 0700) //nolint:gosec // need +x for shared library
}

func extractFromTgz(archivePath, target, dest string) error {
	f, err := os.Open(archivePath) //nolint:gosec // app-controlled path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Name == target || strings.HasSuffix(h.Name, "/"+filepath.Base(target)) {
			out, err := os.Create(dest) //nolint:gosec // app-controlled path
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(tr, maxDecompressSize))
			_ = out.Close()
			return err
		}
	}
	return fmt.Errorf("not found in archive: %s", target)
}

func extractFromZip(archivePath, target, dest string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, zf := range r.File {
		if zf.Name == target || strings.HasSuffix(zf.Name, "/"+filepath.Base(target)) {
			src, err := zf.Open()
			if err != nil {
				return err
			}
			defer func() { _ = src.Close() }()
			out, err := os.Create(dest) //nolint:gosec // app-controlled path
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(src, maxDecompressSize))
			_ = out.Close()
			return err
		}
	}
	return fmt.Errorf("not found in archive: %s", target)
}

type progressReader struct {
	reader   io.Reader
	total    int64
	current  int64
	name     string
	callback ProgressFunc
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.reader.Read(b)
	p.current += int64(n)
	if p.callback != nil {
		p.callback(p.name, p.current, p.total)
	}
	return n, err
}
