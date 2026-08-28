// invariant.go 是 ipc 包的不变量 companion：校验领域表完整性。
//
// AllDomains 是 IPC 分类的唯一权威数据（Classify/提示词注入都读它）：
// 八个大类必须齐全、关键词表非空且无重复——表缺项会导致分类静默漂移
// （缺类的文本全部落入默认 IPCB），故以运行时检查兜底。

package ipc

import (
	"fmt"

	"github.com/xujian519/mady/invariant"
)

// requiredSections 是 IPC 分类法规定的八个大类。
var requiredSections = []IPCSection{IPCA, IPCB, IPCC, IPCD, IPCE, IPCF, IPCG, IPCH}

func init() {
	invariant.Register(invariant.Check{
		Package: "domains/ipc",
		Name:    "all-domains-completeness",
		Fn:      checkAllDomainsCompleteness,
	})
}

// checkAllDomainsCompleteness 校验 AllDomains：
//  1. A-H 八个大类全部存在；
//  2. 每个大类关键词表非空且无空字符串；
//  3. 每个大类内无重复关键词（重复会虚增命中数，扭曲置信度校准）。
func checkAllDomainsCompleteness() error {
	var errs []string
	for _, sec := range requiredSections {
		domain, ok := AllDomains[sec]
		if !ok {
			errs = append(errs, fmt.Sprintf("缺少大类 %s", sec))
			continue
		}
		if len(domain.Keywords) == 0 {
			errs = append(errs, fmt.Sprintf("大类 %s 关键词表为空", sec))
			continue
		}
		seen := make(map[string]bool, len(domain.Keywords))
		for _, kw := range domain.Keywords {
			if kw == "" {
				errs = append(errs, fmt.Sprintf("大类 %s 含空关键词", sec))
				continue
			}
			if seen[kw] {
				errs = append(errs, fmt.Sprintf("大类 %s 关键词重复: %q", sec, kw))
			}
			seen[kw] = true
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("IPC 领域表完整性破坏: %v", errs)
	}
	return nil
}
