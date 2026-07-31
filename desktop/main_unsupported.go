//go:build linux || windows

// 非 macOS 平台占位入口（阶段 4 预留，见 docs/specs/desktop/04-tasks.md
// T4.1 Windows / T4.2 Linux）。
//
// 桌面端当前仅支持 macOS（全部功能文件带 //go:build darwin 约束）。
// 其他平台构建会编译本文件并给出清晰错误，而不是晦涩的 undefined reference。
// 适配工作排期见 docs/plans/desktop-next-development-plan.md。
package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	if _, err := fmt.Fprintf(os.Stderr, "[mady-desktop] 当前版本不支持 %s：桌面端处于 macOS-only 阶段（计划 T4.1/T4.2）\n", runtime.GOOS); err != nil {
		// stderr 写入失败无其他恢复路径，直接退出
		os.Exit(1)
	}
	os.Exit(1)
}
