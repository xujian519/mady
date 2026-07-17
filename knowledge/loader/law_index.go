package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/xujian519/mady/pkg/lawcite"
)

// 本文件实现 S2 知识库法条索引（docs/design/citation-verification-gate.md
// §5 决策二 / §9 P2a）：从 wiki 拆分法条 markdown 的 H3 标题
// （### 第X条 <标题>）构建「条号 → 主题关键词」索引，供引用核验 Gate
// 在 S1 内嵌静态表之外获得运行时主题源。
//
// 依赖方向：本包只暴露数据方法，不 import guardrails；装配侧（cmd/mady）
// 用 guardrails.CitationSourceFuncs 适配后与 S1 静态表经
// guardrails.CompositeCitationSource 组合注入，符合依赖倒置（设计 §7）。
//
// v1 范围限定《专利法（2020）》拆分文件：实施细则-2023 因 2001/2010/2023
// 版本间条号漂移（考试答案多按旧口径引用，用 2023 主题核验必误报）
// 暂不启用，留待 P3+ 做版本感知索引。

// LawArticleIndex 是单部法律的「条号 → 主题关键词」索引。
// 构建完成后只读，并发安全（核验在 AfterModelCall 热路径上并发消费）。
type LawArticleIndex struct {
	topics     map[int][]string
	maxArticle int
}

// lawHeadingRe 匹配 wiki 拆分法条中的 H3 法条标题：### 第X条 <标题>。
// 数字字符集与 lawcite.chineseToArabic 支持的字符集保持一致。
var lawHeadingRe = regexp.MustCompile(`^###\s*第([〇零一二两三四五六七八九十百千]+)条\s+(.+?)\s*$`)

// lawFilePrefix 限定 v1 只索引《专利法（2020）》拆分文件。
const lawFilePrefix = "专利法-2020-拆分-"

// topicSeparators 把 H3 标题按中文连词切分为子短语。
// 例："禁止重复授权与先申请原则" → ["禁止重复授权", "先申请原则"]。
var topicSeparators = strings.NewReplacer(
	"与", " ", "及", " ", "和", " ", "、", " ",
)

// BuildLawArticleIndex 遍历 wikiLegalDir 下「专利法-2020-拆分-*.md」文件
// （排除文件名含「目录」的索引文件），解析 H3 法条标题构建索引。
//
// 同目录的 -part 分卷文件与完整版「专利法-2020.md」不含 H3 法条标题
// （已实测 grep 计数为 0），天然不参与，无重复计数风险。
// 目录不存在或未能索引到任何法条时返回错误——装配侧应降级为仅 S1 源，
// 而不是带病启动。
func BuildLawArticleIndex(wikiLegalDir string) (*LawArticleIndex, error) {
	entries, err := os.ReadDir(wikiLegalDir)
	if err != nil {
		return nil, fmt.Errorf("read law wiki dir %s: %w", wikiLegalDir, err)
	}
	idx := &LawArticleIndex{topics: make(map[int][]string)}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, lawFilePrefix) ||
			!strings.HasSuffix(name, ".md") || strings.Contains(name, "目录") {
			continue
		}
		if err := idx.indexFile(filepath.Join(wikiLegalDir, name)); err != nil {
			return nil, err
		}
	}
	if len(idx.topics) == 0 {
		return nil, fmt.Errorf("no law articles indexed under %s", wikiLegalDir)
	}
	return idx, nil
}

// indexFile 解析单个拆分文件，把其中的 H3 法条标题并入索引。
func (idx *LawArticleIndex) indexFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		m := lawHeadingRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		article, ok := lawcite.ParseChineseNumber(m[1])
		if !ok {
			continue
		}
		idx.topics[article] = mergeKeywords(idx.topics[article], splitTitleTopics(m[2]))
		if article > idx.maxArticle {
			idx.maxArticle = article
		}
	}
	return nil
}

// splitTitleTopics 把 H3 标题切成主题关键词：整串 + 按「与/及/和/、」
// 切分的子短语（≥2 字，去重）。
//
// 这些词只用于「本条自证」（verifyOne 的本条主题命中循环）：词表越宽，
// 正确引用越容易自证通过；交叉匹配（张冠李戴判定）只查 S1 精校词，
// 本函数产出的自动词不参与，因此加词不会引入误报（设计 §6 误报防线）。
func splitTitleTopics(title string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		// ≥2 字：单字词区分度太低（"法""权"等），不作为自证关键词。
		if utf8.RuneCountInString(s) < 2 || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(title)
	for _, sub := range strings.Fields(topicSeparators.Replace(title)) {
		add(sub)
	}
	return out
}

// mergeKeywords 合并两组关键词（去重，保持原有顺序）。
func mergeKeywords(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, kw := range base {
		if !seen[kw] {
			seen[kw] = true
			out = append(out, kw)
		}
	}
	for _, kw := range extra {
		if !seen[kw] {
			seen[kw] = true
			out = append(out, kw)
		}
	}
	return out
}

// Topics 返回该条注册的主题关键词；未覆盖时 ok=false（核验落 Unknown 放行）。
func (idx *LawArticleIndex) Topics(article int) ([]string, bool) {
	kw, ok := idx.topics[article]
	return kw, ok
}

// MaxArticle 返回索引到的最大条号（0 = 空索引，不做存在性核验）。
func (idx *LawArticleIndex) MaxArticle() int { return idx.maxArticle }

// ArticleCount 返回覆盖的条数（供装配侧打日志与冒烟断言，如专利法-2020 应为 82）。
func (idx *LawArticleIndex) ArticleCount() int { return len(idx.topics) }
