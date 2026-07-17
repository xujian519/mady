package guardrails

import "github.com/xujian519/mady/pkg/lawcite"

// 本文件定义引用核验的主题知识源抽象（docs/design/citation-verification-gate.md
// §5 决策二：三层核验源降级）。
//
// S1 内嵌静态表（citation_table.go）编译进二进制，是零依赖默认源；
// S2 知识库法条索引（knowledge/loader.BuildLawArticleIndex，P2）在运行时
// 从 wiki 拆分法条构建，经本接口注入——guardrails 不 import knowledge，
// 由装配侧（cmd/mady）组合，符合依赖倒置（设计 §7）。

// CitationSource 是引用核验的主题知识源。
// 实现必须并发安全（核验在 AfterModelCall 热路径上可能被多 Agent 并发调用）。
type CitationSource interface {
	// Topics 返回该法条的注册主题关键词；未覆盖时 ok=false（核验落 Unknown 放行）。
	Topics(s lawcite.Statute, article int) (keywords []string, ok bool)
	// MaxArticle 返回该法的存在性上限（0 = 不做存在性核验）。
	MaxArticle(s lawcite.Statute) int
}

// staticTableSource 是 S1 内嵌静态主题表实现（citation_table.go）。
type staticTableSource struct{}

func (staticTableSource) Topics(s lawcite.Statute, article int) ([]string, bool) {
	topics, _ := citationTopics(s)
	if topics == nil {
		return nil, false
	}
	kw, ok := topics[article]
	return kw, ok
}

func (staticTableSource) MaxArticle(s lawcite.Statute) int {
	_, maxArticle := citationTopics(s)
	return maxArticle
}

// DefaultCitationSource 返回默认的 S1 内嵌静态表知识源。
// 导出供装配侧与 S2 知识库索引经 CompositeCitationSource 组合：
// CompositeCitationSource(DefaultCitationSource(), s2adapter)。
func DefaultCitationSource() CitationSource { return staticTableSource{} }

// defaultCitationSource 是未注入外部源时的默认实现（S1 静态表）。
func defaultCitationSource() CitationSource { return DefaultCitationSource() }

// CompositeCitationSource 合并两个知识源：关键词取并集（去重，primary 在前），
// 存在性上限取较大非零值。任一源未覆盖时另一源仍可作答——
// S1 手工精校词与 S2 标题词互补，合并语义经回放校准（replay_citation_gate）。
func CompositeCitationSource(primary, secondary CitationSource) CitationSource {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return compositeSource{primary: primary, secondary: secondary}
}

type compositeSource struct {
	primary, secondary CitationSource
}

func (c compositeSource) Topics(s lawcite.Statute, article int) ([]string, bool) {
	pk, pok := c.primary.Topics(s, article)
	sk, sok := c.secondary.Topics(s, article)
	if !pok {
		return sk, sok
	}
	if !sok {
		return pk, true
	}
	seen := make(map[string]bool, len(pk)+len(sk))
	merged := make([]string, 0, len(pk)+len(sk))
	for _, kw := range pk {
		if !seen[kw] {
			seen[kw] = true
			merged = append(merged, kw)
		}
	}
	for _, kw := range sk {
		if !seen[kw] {
			seen[kw] = true
			merged = append(merged, kw)
		}
	}
	return merged, true
}

func (c compositeSource) MaxArticle(s lawcite.Statute) int {
	if m := c.primary.MaxArticle(s); m > 0 {
		return m
	}
	return c.secondary.MaxArticle(s)
}

// CitationSourceFuncs 函数适配器：把普通函数包装成 CitationSource。
// 供 knowledge/loader 的 S2 索引在不 import guardrails 的情况下被装配侧接入。
type CitationSourceFuncs struct {
	TopicsFunc     func(s lawcite.Statute, article int) ([]string, bool)
	MaxArticleFunc func(s lawcite.Statute) int
}

func (f CitationSourceFuncs) Topics(s lawcite.Statute, article int) ([]string, bool) {
	if f.TopicsFunc == nil {
		return nil, false
	}
	return f.TopicsFunc(s, article)
}

func (f CitationSourceFuncs) MaxArticle(s lawcite.Statute) int {
	if f.MaxArticleFunc == nil {
		return 0
	}
	return f.MaxArticleFunc(s)
}
