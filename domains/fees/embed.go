package fees

import (
	"embed"
	"sync"
)

// 随包费用索引。金额与规则逐条记录来源与核验日期，未转录的条目在计价时拒算。
//
//go:embed data/cn-fees.yaml
var bundledTableYAML embed.FS

var (
	bundledOnce  sync.Once
	bundledTable *Table
	bundledErr   error
)

// BundledTable 返回随包分发的费用索引（进程内只解析一次）。
func BundledTable() (*Table, error) {
	bundledOnce.Do(func() {
		data, err := bundledTableYAML.ReadFile("data/cn-fees.yaml")
		if err != nil {
			bundledErr = err
			return
		}
		bundledTable, bundledErr = LoadTable(data, "data/cn-fees.yaml")
	})
	return bundledTable, bundledErr
}
