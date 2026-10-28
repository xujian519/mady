package analysisreport

import (
	"strings"
	"unicode/utf8"
)

// 权利要求技术特征的确定性抽取（v1 以权利要求为粒度）：
// type 由技术领域关键词启发式判定，importance 由权项位置与实质限定长度判定。
// 更细粒度的要素拆分由 claim-chart 引擎承担（其要素必须是权利要求原文的连续子串），
// 两者互补而非重复。

type typeKeywords struct {
	featureType AnalysisFeatureType
	keywords    []string
}

// byPriority 按优先级顺序匹配（先命中先得；"控制/处理/特征"等歧义词不参与）。
var typeKeywordsByPriority = []typeKeywords{
	{FeatureMethod, []string{"方法", "步骤", "工艺", "流程", "检测", "制造", "制备", "判断", "加工"}},
	{FeatureMaterial, []string{"材料", "组合物", "配方", "合金", "涂料", "化合物", "聚合物", "混合物", "制剂", "溶液", "助剂"}},
	{FeatureData, []string{"数据", "信号", "信息", "算法", "模型", "神经网络", "图像", "指令", "参数"}},
	{FeatureSystem, []string{"系统", "平台", "装置", "设备", "整机"}},
	{FeatureDevice, []string{"部件", "组件", "机构", "构件", "支架", "齿轮", "传感器", "阀", "泵", "壳体"}},
}

const (
	typeHitConfidence      = 0.9
	typeFallbackConfidence = 0.5
	// dependentSubstantialMin 是从属权利要求视为「足够实质限定」的字数下限。
	dependentSubstantialMin = 20
)

// DetermineFeatureType 依据技术关键词判定特征类型；无命中时为 other。
func DetermineFeatureType(text string) AnalysisFeatureType {
	for _, entry := range typeKeywordsByPriority {
		for _, keyword := range entry.keywords {
			if strings.Contains(text, keyword) {
				return entry.featureType
			}
		}
	}
	return FeatureOther
}

// DetermineImportance 依据权项位置与实质限定长度判定重要性：权 1 为 high，
// 从属权项达到字数下限为 medium，否则 low。
func DetermineImportance(claimNo int, text string) FeatureImportance {
	if claimNo == 1 {
		return ImportanceHigh
	}
	if utf8.RuneCountInString(text) >= dependentSubstantialMin {
		return ImportanceMedium
	}
	return ImportanceLow
}

// ExtractTechnicalFeatures 从权利要求列表抽取技术特征（claimNo 从 1 起）。
func ExtractTechnicalFeatures(claims []string) []ExtractedFeature {
	features := make([]ExtractedFeature, 0, len(claims))
	for index, text := range claims {
		claimNo := index + 1
		featureType := DetermineFeatureType(text)
		confidence := typeHitConfidence
		if featureType == FeatureOther {
			confidence = typeFallbackConfidence
		}
		features = append(features, ExtractedFeature{
			ClaimNo:    claimNo,
			Text:       text,
			Type:       featureType,
			Importance: DetermineImportance(claimNo, text),
			Confidence: confidence,
		})
	}
	return features
}

// ComputeFeatureStatistics 统计特征分布（by_type / by_importance 含全枚举键，0 值也出现）。
func ComputeFeatureStatistics(features []ExtractedFeature) FeatureStatistics {
	stats := FeatureStatistics{
		ByType: map[AnalysisFeatureType]int{
			FeatureDevice: 0, FeatureMethod: 0, FeatureMaterial: 0,
			FeatureSystem: 0, FeatureData: 0, FeatureOther: 0,
		},
		ByImportance: map[FeatureImportance]int{
			ImportanceHigh: 0, ImportanceMedium: 0, ImportanceLow: 0,
		},
		Total: len(features),
	}
	for _, feature := range features {
		stats.ByType[feature.Type]++
		stats.ByImportance[feature.Importance]++
	}
	return stats
}
