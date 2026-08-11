package chemistry

import (
	"reflect"
	"strings"
	"testing"
)

func TestIsValidHillFormula(t *testing.T) {
	cases := []struct {
		formula string
		want    bool
	}{
		{"C6H12O6", true},
		{"H2SO4", true},
		{"NaCl", true},
		{"CH4", true},
		{"  CH4  ", true},
		{"C12", false}, // 单元素
		{"N2", false},  // 单元素
		{"CD4", false}, // D 非元素
		{"DNA", false},
		{"ATP", false},
		{"ABC", false},   // AB 非元素
		{"C1-C6", false}, // 含非法字符
		{"", false},
		{"!@#", false},
		{strings.Repeat("CH4", 50), false}, // 超长（150 > 128）
		{strings.Repeat("CH4", 30), true},  // 90 字符仍合法
	}
	for _, c := range cases {
		if got := IsValidHillFormula(c.formula); got != c.want {
			t.Errorf("IsValidHillFormula(%q) = %v, want %v", c.formula, got, c.want)
		}
	}
}

func TestExtractFormulaCandidates(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"该水合物分子式为C6H12O6，另含少量H2SO4。", []string{"C6H12O6", "H2SO4"}},
		{"甲烷CH4与氯化钠NaCl反应。", []string{"CH4", "NaCl"}},
		{"见C12章节及式(I)所示结构。", []string{}},       // C12/式(I) 均为单元素噪声
		{"细胞中的DNA与ATP参与代谢，CD4阳性。", []string{}}, // 生物医药缩略词
		{"浓度范围C1-C6的烷烃。", []string{}},          // 范围写法
		{"太阳能装置含6个光伏板。", []string{}},
		{"", []string{}},
	}
	for _, c := range cases {
		if got := ExtractFormulaCandidates(c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExtractFormulaCandidates(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestExtractSmilesCandidates(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"化合物结构为c1ccccc1。", []string{"c1ccccc1"}},
		{"双环芳烃c1ccc2ccccc2c1。", []string{"c1ccc2ccccc2c1"}},
		{"中间体CC(=O)O参与反应。", []string{"CC(=O)O"}},
		{"比例M2=5不符合，ratio=2亦非。", []string{}}, // M 非元素；r 非芳环原子
		{"氢氧化钠NaOH是强碱。", []string{}},         // 无结构要素，非 SMILES 词元
		{"乙醇CCO是。", []string{}},              // 长度 3 < 4
		{"", []string{}},
	}
	for _, c := range cases {
		if got := ExtractSmilesCandidates(c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExtractSmilesCandidates(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestExtractChemicalCandidates(t *testing.T) {
	got := ExtractChemicalCandidates("该药含C6H12O6与c1ccccc1结构。")
	want := ChemicalExtraction{
		Formulas:     []string{"C6H12O6"},
		SmilesTokens: []string{"c1ccccc1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractChemicalCandidates() = %+v, want %+v", got, want)
	}
}
