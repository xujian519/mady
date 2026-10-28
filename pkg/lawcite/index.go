package lawcite

import (
	"embed"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/xujian519/mady/pkg/indexasset"
)

// 随包法条索引。两份资产都是从官方全文逐字转录的子集，条目自带来源与核验日期；
// 未转录的条号只作为骨架存在，核验时判 StatusExistOnly 而不是 StatusVerified。
//
//go:embed data/law/cn-patent-law.yaml data/law/cn-implementing-regulations.yaml
var lawAssets embed.FS

// 随包索引文件名，按 Statute 映射。
const (
	patentLawAsset         = "data/law/cn-patent-law.yaml"
	implementingRulesAsset = "data/law/cn-implementing-regulations.yaml"
)

// Status 是法条引用的核验结论。
//
// 结论与「怎么处置」分离：消费方按自己的策略决定 block/warn，本包只负责判定。
type Status int

const (
	// StatusUnverified 表示无法判定：该法未收录，或条号超出未经核验的上限。
	StatusUnverified Status = iota
	// StatusExistOnly 表示条号确认存在，但条文尚未转录，不能据此确认内容。
	StatusExistOnly
	// StatusVerified 表示条文已转录，且来源与核验日期齐备。
	StatusVerified
	// StatusOutOfRange 表示条号超出已核验的条数上限。
	StatusOutOfRange
	// StatusNarrowingMismatch 表示款/项超出该条已登记的范围。
	StatusNarrowingMismatch
)

// String 返回状态的中文名，用于报告与日志。
func (s Status) String() string {
	switch s {
	case StatusExistOnly:
		return "条号存在但未转录"
	case StatusVerified:
		return "已核验"
	case StatusOutOfRange:
		return "条号越界"
	case StatusNarrowingMismatch:
		return "款项越界"
	default:
		return "未核验"
	}
}

// Article 是索引中的一条法条。
type Article struct {
	// Article 是条号。
	Article int
	// Topics 是命题匹配用的主题词。
	Topics []string
	// Paragraphs 是已登记存在的款号；空表示未登记。
	Paragraphs []int
	// Text 是条文原文；未转录时为空。
	Text string
	// SourceDoc 是条文抄录依据的官方文书。
	SourceDoc string
	// VerifiedOn 是人工核验日期。
	VerifiedOn string
}

// Index 是一份法条索引。
type Index struct {
	// Statute 是该索引对应的法律文件。
	Statute Statute
	// Law 是法律简称（如「专利法」），用于提示文案。
	Law string
	// Document 是法律全称。
	Document string
	// Revision 是修订版本。
	Revision string
	// MaxArticle 是条数上限；0 表示资产未声明。
	MaxArticle int
	// MaxVerifiedOn 是条数上限的核验日期；空表示上限本身尚未核验。
	MaxVerifiedOn string
	// MaxSource 是条数上限的核验依据。
	MaxSource string

	articles map[int]Article
}

// Resolution 是一次引用的核验结果。
type Resolution struct {
	// Citation 是被核验的引用。
	Citation Citation
	// Status 是核验结论。
	Status Status
	// Article 是命中的条目；索引无该条时为 nil。
	Article *Article
	// Reason 是面向用户的一句话说明，含法律名与条号。
	Reason string
}

// LoadIndex 从资产文本加载一份法条索引。
func LoadIndex(statute Statute, data []byte, origin string) (*Index, error) {
	fail := indexasset.FailFor(origin)
	root, err := indexasset.ParseYAMLMapping(data, "法条索引", fail)
	if err != nil {
		return nil, err
	}
	if err := indexasset.RequireKnownFields(root,
		[]string{"law", "document", "revision", "maxArticle", "maxVerifiedOn", "maxSource", "verifiedOn", "articles"},
		"法条索引根节点", fail); err != nil {
		return nil, err
	}
	law, err := indexasset.ReadString(root, "law", fail)
	if err != nil {
		return nil, err
	}
	document, err := indexasset.ReadString(root, "document", fail)
	if err != nil {
		return nil, err
	}
	revision, err := indexasset.ReadString(root, "revision", fail)
	if err != nil {
		return nil, err
	}
	maxArticle, err := indexasset.ReadOptionalCount(root, "maxArticle", fail)
	if err != nil {
		return nil, err
	}
	maxVerifiedOn, err := indexasset.ReadOptionalDate(root, "maxVerifiedOn", fail)
	if err != nil {
		return nil, err
	}
	maxSource, err := indexasset.ReadOptionalString(root, "maxSource", fail)
	if err != nil {
		return nil, err
	}
	entries, err := indexasset.ReadArticleEntries(root["articles"], fail)
	if err != nil {
		return nil, err
	}

	idx := &Index{
		Statute:  statute,
		Law:      law,
		Document: document,
		Revision: revision,
		articles: make(map[int]Article, len(entries)),
	}
	if maxArticle != nil {
		idx.MaxArticle = *maxArticle
	}
	if maxVerifiedOn != nil {
		idx.MaxVerifiedOn = *maxVerifiedOn
	}
	if maxSource != nil {
		idx.MaxSource = *maxSource
	}
	for _, entry := range entries {
		article := Article{
			Article:    entry.Article,
			Topics:     entry.Topics,
			Paragraphs: entry.Paragraphs,
		}
		if entry.Text != nil {
			article.Text = *entry.Text
		}
		if entry.SourceDoc != nil {
			article.SourceDoc = *entry.SourceDoc
		}
		if entry.VerifiedOn != nil {
			article.VerifiedOn = *entry.VerifiedOn
		}
		idx.articles[article.Article] = article
	}
	return idx, nil
}

// Article 返回条号命中的条目。
func (idx *Index) Article(number int) (Article, bool) {
	if idx == nil {
		return Article{}, false
	}
	article, ok := idx.articles[number]
	return article, ok
}

// Articles 按条号升序返回全部条目。
func (idx *Index) Articles() []Article {
	if idx == nil {
		return nil
	}
	articles := make([]Article, 0, len(idx.articles))
	for _, article := range idx.articles {
		articles = append(articles, article)
	}
	slices.SortFunc(articles, func(a, b Article) int { return a.Article - b.Article })
	return articles
}

// Resolve 核验一条引用。
//
// 判定顺序：条号越界 → 未收录 → 未转录 → 款项越界 → 已核验。
// 越界只有在条数上限本身已核验时才成立；上限未核验时降级为未核验，
// 避免用未核验的数字断言某条不存在。
func (idx *Index) Resolve(c Citation) Resolution {
	if idx == nil {
		return Resolution{Citation: c, Status: StatusUnverified, Reason: "未提供该法律的索引，未做核验"}
	}
	if c.Article <= 0 {
		return Resolution{Citation: c, Status: StatusUnverified, Reason: "条号不合法，未做核验"}
	}

	entry, found := idx.articles[c.Article]
	if !found {
		if idx.MaxArticle > 0 && c.Article > idx.MaxArticle {
			if idx.MaxVerifiedOn != "" {
				return Resolution{
					Citation: c,
					Status:   StatusOutOfRange,
					Reason:   fmt.Sprintf("《%s》共 %d 条，第 %d 条不存在", idx.Law, idx.MaxArticle, c.Article),
				}
			}
			return Resolution{
				Citation: c,
				Status:   StatusUnverified,
				Reason: fmt.Sprintf("第 %d 条超出索引声明的上限 %d 条，但该上限本身尚未核验",
					c.Article, idx.MaxArticle),
			}
		}
		if idx.MaxArticle > 0 && idx.MaxVerifiedOn != "" {
			return Resolution{
				Citation: c,
				Status:   StatusExistOnly,
				Reason: fmt.Sprintf("《%s》第 %d 条条号存在，但条文尚未收录，不能据此确认内容",
					idx.Law, c.Article),
			}
		}
		return Resolution{
			Citation: c,
			Status:   StatusUnverified,
			Reason:   fmt.Sprintf("《%s》第 %d 条不在索引中", idx.Law, c.Article),
		}
	}

	if entry.Text == "" || entry.SourceDoc == "" || entry.VerifiedOn == "" {
		return Resolution{
			Citation: c,
			Status:   StatusExistOnly,
			Article:  &entry,
			Reason: fmt.Sprintf("《%s》第 %d 条已索引，但条文尚未转录核验（条目主题：%s）",
				idx.Law, c.Article, joinTopics(entry.Topics)),
		}
	}

	if c.Paragraph > 0 && len(entry.Paragraphs) > 0 && !slices.Contains(entry.Paragraphs, c.Paragraph) {
		return Resolution{
			Citation: c,
			Status:   StatusNarrowingMismatch,
			Article:  &entry,
			Reason: fmt.Sprintf("第 %d 条索引载明共 %d 款，不含第 %d 款",
				c.Article, len(entry.Paragraphs), c.Paragraph),
		}
	}

	return Resolution{
		Citation: c,
		Status:   StatusVerified,
		Article:  &entry,
		Reason:   fmt.Sprintf("与《%s》第 %d 条一致（来源：%s，核验日期：%s）", idx.Law, c.Article, entry.SourceDoc, entry.VerifiedOn),
	}
}

// joinTopics 把主题词拼成提示文案用的短语。
func joinTopics(topics []string) string {
	if len(topics) == 0 {
		return "无"
	}
	return strings.Join(topics, "、")
}

// bundledIndex 是单个法律的随包索引缓存。
type bundledIndex struct {
	once sync.Once
	idx  *Index
	err  error
}

// bundledIndexes 缓存各法律的随包索引。
var bundledIndexes sync.Map // Statute -> *bundledIndex

// BundledIndex 返回随包分发的该法律索引。
//
// 没有随包索引的法律返回错误；调用方应把该错误映射为 StatusUnverified，
// 而不是当作「该条不存在」。
func BundledIndex(statute Statute) (*Index, error) {
	asset, ok := bundledAsset(statute)
	if !ok {
		return nil, fmt.Errorf("lawcite: %s 没有随包索引", statute)
	}
	cached, _ := bundledIndexes.LoadOrStore(statute, &bundledIndex{})
	entry := cached.(*bundledIndex)
	entry.once.Do(func() {
		data, err := lawAssets.ReadFile(asset)
		if err != nil {
			entry.err = fmt.Errorf("lawcite: 读取随包索引 %s: %w", asset, err)
			return
		}
		entry.idx, entry.err = LoadIndex(statute, data, asset)
	})
	return entry.idx, entry.err
}

// bundledAsset 返回法律对应的随包资产路径。
func bundledAsset(statute Statute) (string, bool) {
	switch statute {
	case StatutePatentLaw:
		return patentLawAsset, true
	case StatuteImplementingRules:
		return implementingRulesAsset, true
	default:
		return "", false
	}
}
