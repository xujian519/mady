package plantask

import "testing"

// TestInvariant_TransitionMatrixClosure 是 companion 检查的直接用例：
// 当前内置矩阵必须通过闭合性校验。
func TestInvariant_TransitionMatrixClosure(t *testing.T) {
	if err := checkTransitionMatrixClosure(); err != nil {
		t.Fatalf("built-in transition matrix violates closure: %v", err)
	}
}
