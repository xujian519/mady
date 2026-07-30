// Package subcmd 是 Mady 子命令实现包。
//
// 本包将 cmd/mady 中除 TUI、serve、acp 之外的所有子命令提取为独立文件，
// 遵循「cmd 只放 main」原则，使 cmd/mady 保持简洁。
//
// 子命令：
//   - evidence: 证据判断 CLI
//   - eval: 评估套件运行器
//   - patent: 专利分析 CLI（新颖性/OA/无效/侵权/复审）
//   - mcp-install: MCP 安装器
//   - trust-mcp: MCP 配置信任
//   - trust-knowledge: 知识库沙箱白名单管理
//   - ocr: 本地 OCR CLI
//   - util: 实用工具（list-prompts）
package subcmd
