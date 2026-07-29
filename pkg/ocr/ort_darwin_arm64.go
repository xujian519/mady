//go:build darwin && arm64

package ocr

const (
	ortLibName        = "onnxruntime_arm64.dylib"
	ortDownloadURL    = "https://github.com/microsoft/onnxruntime/releases/download/v1.24.4/onnxruntime-osx-arm64-1.24.4.tgz"
	ortArchiveLibPath = "onnxruntime-osx-arm64-1.24.4/lib/libonnxruntime.1.24.4.dylib"
	ortArchiveFormat  = "tgz"
)
