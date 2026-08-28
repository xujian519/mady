// invariant.go 是 plantask 的不变量 companion：校验状态迁移矩阵的闭合性。
//
// 迁移矩阵是 plantask 全部状态流转的权威定义（02-spec §2.2 白名单），
// 表内出现"终态被当作源状态""未知状态作为目标"或"非终态无出边"都意味着
// 表被破坏——这类破坏在编译期不可见（map 字面量），故以运行时检查兜底。

package plantask

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xujian519/mady/invariant"
)

func init() {
	invariant.Register(invariant.Check{
		Package: "agentcore/plantask",
		Name:    "transition-matrix-closure",
		Fn:      checkTransitionMatrixClosure,
	})
}

// checkTransitionMatrixClosure 校验迁移矩阵：
//  1. 所有源状态均为非终态（终态不参与任何迁移）；
//  2. 每个源状态至少有一条出边；
//  3. 所有目标状态均为已知状态（出现过作为源，或为终态）。
func checkTransitionMatrixClosure() error {
	// 收集全部已知状态：矩阵源 + 各目标 + 终态。
	known := map[Status]bool{}
	for from, dsts := range transitionMatrix {
		known[from] = true
		for to := range dsts {
			known[to] = true
		}
	}
	known[StatusExpired] = true

	var errs []string
	for from, dsts := range transitionMatrix {
		if isTerminal(from) {
			errs = append(errs, fmt.Sprintf("终态 %q 出现在迁移矩阵的源位置", from))
		}
		if len(dsts) == 0 {
			errs = append(errs, fmt.Sprintf("非终态 %q 无出边（陷入死状态）", from))
		}
		for to := range dsts {
			if !known[to] {
				errs = append(errs, fmt.Sprintf("迁移 %q→%q 的目标为未知状态", from, to))
			}
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("迁移矩阵闭合性破坏: %s", strings.Join(errs, "; "))
	}
	return nil
}
