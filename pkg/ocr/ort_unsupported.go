//go:build !(darwin && arm64) && !(linux && amd64) && !(linux && arm64)

package ocr

const (
	ortLibName        = ""
	ortDownloadURL    = ""
	ortArchiveLibPath = ""
	ortArchiveFormat  = ""
)
