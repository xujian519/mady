package iface

import (
	"testing"
)

// TestInterfaceContract 验证接口的编译期契约。
// 这些测试通过类型断言确保结构体实现接口，不依赖具体实例。
func TestInterfaceContract(t *testing.T) {
	// 验证 BaseLifecycleHook 实现了 LifecycleHook 接口
	var _ LifecycleHook = BaseLifecycleHook{}
}
