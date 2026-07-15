// Package fileindex 提供文件索引器，支持从 PDF、DOCX、图片、音频、
// 电子表格和纯文本等多文件类型中提取文本内容用于知识库索引。
//
// 支持的格式：
//   - PDF: 文本提取与 OCR 回退
//   - DOCX: Office Open XML 解析
//   - 图片: EXIF/Tesseract OCR
//   - 音频: 语音转文本（Whisper）
//   - 电子表格: CSV/XLSX 文本提取
//   - 纯文本: TXT/MD/代码文件
//
// 主要类型：
//   - Extension: 文件类型注册器，按扩展名分发到对应读取器
//   - Reader: 文件读取器接口（ReaderPDF / ReaderDOCX / ReaderImage 等）
//   - Result: 文本提取结果（内容 + 元数据）
//
// 使用示例：
//
//	ext := fileindex.NewExtension()
//	content, _ := ext.ReadFile(ctx, "patent.pdf")
package fileindex
