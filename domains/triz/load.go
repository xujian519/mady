package triz

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

// TRIZ 随包资产：39x39 经典矛盾矩阵与 40 条发明原理。
// 资产随二进制嵌入，缺文件或格式错误在首次使用时暴露为明确错误，不做静默降级。

//go:embed assets/triz-matrix.json assets/triz-principles.json
var assets embed.FS

// MatrixMin / MatrixMax 是工程参数编号范围。
const (
	MatrixMin = 1
	MatrixMax = 39
)

// principleAsset 是随包原理资产的线上形状（编号字段名为 no）。
type principleAsset struct {
	No          int    `json:"no"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var (
	loadOnce       sync.Once
	matrix         [][][]int
	principles     []Principle
	principleIndex map[int]Principle
	loadErr        error
)

// loadAssets 读取并校验随包资产（只执行一次）。
func loadAssets() error {
	loadOnce.Do(func() {
		var rawMatrix [][][]int
		if err := readAsset("assets/triz-matrix.json", &rawMatrix); err != nil {
			loadErr = err
			return
		}
		if err := validateMatrix(rawMatrix); err != nil {
			loadErr = err
			return
		}
		var rawPrinciples []principleAsset
		if err := readAsset("assets/triz-principles.json", &rawPrinciples); err != nil {
			loadErr = err
			return
		}
		loaded := make([]Principle, 0, len(rawPrinciples))
		for _, asset := range rawPrinciples {
			loaded = append(loaded, Principle{
				Number: asset.No, Name: asset.Name, Description: asset.Description,
			})
		}
		if err := validatePrinciples(loaded); err != nil {
			loadErr = err
			return
		}
		index := make(map[int]Principle, len(loaded))
		for _, principle := range loaded {
			index[principle.Number] = principle
		}
		matrix = rawMatrix
		principles = loaded
		principleIndex = index
	})
	return loadErr
}

func readAsset(name string, target any) error {
	data, err := assets.ReadFile(name)
	if err != nil {
		return fmt.Errorf("triz: 读取随包资产 %s 失败: %w", name, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("triz: 解析随包资产 %s 失败: %w", name, err)
	}
	return nil
}

func validateMatrix(m [][][]int) error {
	if len(m) != MatrixMax {
		return fmt.Errorf("triz: 矛盾矩阵应有 %d 行，实际 %d 行", MatrixMax, len(m))
	}
	for rowIndex, row := range m {
		if len(row) != MatrixMax {
			return fmt.Errorf("triz: 矛盾矩阵第 %d 行应有 %d 列，实际 %d 列", rowIndex+1, MatrixMax, len(row))
		}
		for colIndex, cell := range row {
			for _, no := range cell {
				if no < 1 || no > 40 {
					return fmt.Errorf("triz: 矛盾矩阵格 (%d,%d) 含非法原理编号 %d", colIndex+1, rowIndex+1, no)
				}
			}
		}
	}
	return nil
}

func validatePrinciples(list []Principle) error {
	if len(list) != 40 {
		return fmt.Errorf("triz: 发明原理应有 40 条，实际 %d 条", len(list))
	}
	seen := make(map[int]bool, len(list))
	for _, principle := range list {
		if principle.Number < 1 || principle.Number > 40 {
			return fmt.Errorf("triz: 发明原理编号 %d 超出 1-40", principle.Number)
		}
		if seen[principle.Number] {
			return fmt.Errorf("triz: 发明原理编号 %d 重复", principle.Number)
		}
		if principle.Name == "" || principle.Description == "" {
			return fmt.Errorf("triz: 发明原理 %d 缺少名称或说明", principle.Number)
		}
		seen[principle.Number] = true
	}
	return nil
}

// Principles 返回 40 条发明原理（按编号升序）。
func Principles() []Principle {
	if err := loadAssets(); err != nil {
		return nil
	}
	out := make([]Principle, len(principles))
	copy(out, principles)
	return out
}

// PrincipleByNo 按编号取发明原理。
func PrincipleByNo(no int) (Principle, bool) {
	if err := loadAssets(); err != nil {
		return Principle{}, false
	}
	principle, ok := principleIndex[no]
	return principle, ok
}

// LookupCell 返回「改善 improving、恶化 worsening」在经典矩阵中的推荐原理编号，
// 按资产顺序去重；对角格与转录空缺均返回空切片。
//
// 矩阵按 [worsening-1][improving-1] 索引，与随包资产的行列定义一致。
func LookupCell(improving, worsening int) ([]int, error) {
	if improving < MatrixMin || improving > MatrixMax || worsening < MatrixMin || worsening > MatrixMax {
		return nil, fmt.Errorf("triz: 工程参数编号须在 %d-%d：improving=%d, worsening=%d",
			MatrixMin, MatrixMax, improving, worsening)
	}
	if err := loadAssets(); err != nil {
		return nil, err
	}
	if improving == worsening {
		return nil, nil
	}
	cell := matrix[worsening-1][improving-1]
	seen := make(map[int]bool, len(cell))
	out := make([]int, 0, len(cell))
	for _, no := range cell {
		if seen[no] {
			continue
		}
		seen[no] = true
		out = append(out, no)
	}
	return out, nil
}
