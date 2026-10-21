// Package fileindex 提供项目文件索引与内容读取：基于 SQLite FTS5 的增量索引、
// 文件变更监听，以及按扩展名分发的文本提取。
//
// 支持的格式：
//   - PDF: 调用系统 pdftotext 提取文本
//   - DOCX: 解析 OOXML 正文与页眉页脚
//   - XLSX/CSV: 转换为 Markdown 表格
//   - 纯文本/代码: 直接读取，超大文件按 UTF-8 边界截断
//   - 图片/音频: 仅返回元数据与处理建议（不做 OCR/ASR）
//
// 主要类型：
//   - FileIndex: SQLite 文件索引（增量刷新、四路 RRF 搜索）
//   - FileWatcher: fsnotify 监听 + 轮询回退
//   - FileReader: 项目根目录内的文件读取器（沙箱路径校验）
//   - Extension: agentcore 扩展，注册 read_project_file / search_project_files 工具
//
// 使用示例：
//
//	fr := fileindex.NewFileReader(projectRoot)
//	result, err := fr.ReadProjectFile(ctx, "docs/权利要求.md")
package fileindex
