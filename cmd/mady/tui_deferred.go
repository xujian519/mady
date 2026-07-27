package main

// tui_deferred.go 是 pkg/framework.DeferredInit 的类型别名层。
// 保持 cmd/mady 现有代码的无缝编译。

import "github.com/xujian519/mady/pkg/framework"

// DeferredInit 是 framework.DeferredInit 的类型别名。
type DeferredInit = framework.DeferredInit

// newDeferredInit 委托到 framework.NewDeferredInit。
func newDeferredInit() *DeferredInit {
	return framework.NewDeferredInit()
}
