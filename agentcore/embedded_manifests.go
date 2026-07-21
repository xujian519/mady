package agentcore

import "embed"

// embeddedManifestsDir 是 go:embed 嵌入的 manifest 文件在 embed.FS 中的目录。
// 与本文件同级的 manifests/ 目录（agentcore/manifests/）保持一致。
const embeddedManifestsDir = "manifests"

// embeddedManifestsFS 嵌入了开箱即用的内置领域 manifest（assistant/patent/legal）。
// Chat Agent 已融入集成模式（Invisible Handoff），不再作为独立 manifest 存在。
//
// 这使得 mady 二进制在任意工作目录启动时，都能无需外部资源文件即可加载
// 多域路由所需的领域定义。用户可在 MADY_HOME/manifests/ 放同名文件覆盖，
// 或放新文件新增领域，无需重新编译（见 LoadManifests）。
//
//go:embed manifests/*.json
var embeddedManifestsFS embed.FS
