package guardrails

import (
	"fmt"

	"github.com/xujian519/mady/pkg/lawcite"
)

// IndexCitationSource 用随包法条索引提供主题词、条数上限与条文核验结论。
//
// 与静态表的关系：索引命中的条目优先（它带条文原文与核验日期），索引没有的
// 条号或法律回退静态表——静态表覆盖专利法族之外的商标法、著作权法与反不正当
// 竞争法，但只有主题词，没有条文与来源。因此 `Resolve` 对无索引的法律一律返回
// 未核验，绝不把静态表的主题词当成「条文已核验」。
type IndexCitationSource struct {
	indexes  map[lawcite.Statute]*lawcite.Index
	fallback CitationSource
}

// NewIndexCitationSource 加载随包索引，并以 fallback 作为静态表回退。
//
// 加载失败的法律不会进入索引表，其引用按「未核验」处理；错误通过 IndexErrors
// 暴露给启动期日志，避免静默丢掉一份索引。
func NewIndexCitationSource(fallback CitationSource) (*IndexCitationSource, error) {
	if fallback == nil {
		fallback = DefaultCitationSource()
	}
	source := &IndexCitationSource{
		indexes:  make(map[lawcite.Statute]*lawcite.Index, 2),
		fallback: fallback,
	}
	var errs []error
	for _, statute := range []lawcite.Statute{
		lawcite.StatutePatentLaw,
		lawcite.StatuteImplementingRules,
	} {
		idx, err := lawcite.BundledIndex(statute)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		source.indexes[statute] = idx
	}
	if len(errs) > 0 {
		return source, fmt.Errorf("guardrails: 随包法条索引加载失败：%v", errs)
	}
	return source, nil
}

// Topics 返回条目的主题词：索引主题词与静态表主题词合并去重。
//
// 合并而不是替换：护栏对主题词是「任一命中即视为相关」的语义，少一个词只会
// 增加误报（放行不足），多一个词不改变已能命中的判断。
func (s *IndexCitationSource) Topics(statute lawcite.Statute, article int) ([]string, bool) {
	var primary []string
	var primaryOK bool
	if idx, ok := s.indexes[statute]; ok {
		if entry, found := idx.Article(article); found && len(entry.Topics) > 0 {
			primary, primaryOK = entry.Topics, true
		}
	}
	secondary, secondaryOK := s.fallback.Topics(statute, article)
	if !primaryOK {
		return secondary, secondaryOK
	}
	if !secondaryOK {
		return primary, true
	}
	seen := make(map[string]bool, len(primary)+len(secondary))
	merged := make([]string, 0, len(primary)+len(secondary))
	for _, topics := range [][]string{primary, secondary} {
		for _, topic := range topics {
			if !seen[topic] {
				seen[topic] = true
				merged = append(merged, topic)
			}
		}
	}
	return merged, true
}

// MaxArticle 返回条数上限：索引声明的上限优先，回退静态表。
func (s *IndexCitationSource) MaxArticle(statute lawcite.Statute) int {
	if idx, ok := s.indexes[statute]; ok && idx.MaxArticle > 0 {
		return idx.MaxArticle
	}
	return s.fallback.MaxArticle(statute)
}

// Resolve 核验一条引用。
//
// 没有随包索引的法律返回未核验——静态表只能给出主题词，不足以确认条文内容。
func (s *IndexCitationSource) Resolve(c lawcite.Citation) lawcite.Resolution {
	if idx, ok := s.indexes[c.Statute]; ok {
		return idx.Resolve(c)
	}
	return lawcite.Resolution{
		Citation: c,
		Status:   lawcite.StatusUnverified,
		Reason:   fmt.Sprintf("《%s》尚无随包条文索引，未做核验", c.Statute),
	}
}

// Indexed 返回已加载索引的法律集合，供启动日志与测试使用。
func (s *IndexCitationSource) Indexed() []lawcite.Statute {
	statutes := make([]lawcite.Statute, 0, len(s.indexes))
	for statute := range s.indexes {
		statutes = append(statutes, statute)
	}
	return statutes
}
