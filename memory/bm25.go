package memory

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// BM25Config 控制 BM25 评分算法的参数。
type BM25Config struct {
	K1 float64 // 词频饱和度参数 (default: 1.5)
	B  float64 // 文档长度归一化参数 (default: 0.75)
}

// DefaultBM25Config 返回标准 BM25 参数。
func DefaultBM25Config() BM25Config {
	return BM25Config{K1: 1.5, B: 0.75}
}

// BM25Index 是 BM25 评分的倒排索引。
// 支持增量添加、删除和重建。
type BM25Index struct {
	mu sync.RWMutex

	// 文档存储
	docs map[string]*bm25Doc // entryID → document

	// 倒排索引: token → (entryID → term frequency in doc)
	inverted map[string]map[string]int

	// 文档频率: token → number of docs containing this token
	docFreq map[string]int

	// 文档总数
	docCount int
	// 平均文档长度
	avgDL float64

	cfg BM25Config
}

// bm25Doc 是 BM25 索引中的单条文档。
type bm25Doc struct {
	id     string
	tokens []string
	length int
}

// NewBM25Index 创建一个空的 BM25 索引。
func NewBM25Index(cfg BM25Config) *BM25Index {
	return &BM25Index{
		docs:     make(map[string]*bm25Doc),
		inverted: make(map[string]map[string]int),
		docFreq:  make(map[string]int),
		cfg:      cfg,
	}
}

// Add 添加一条文档到索引。
func (idx *BM25Index) Add(entryID, content string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tokens := bm25Tokenize(content)
	if len(tokens) == 0 {
		return
	}

	doc := &bm25Doc{id: entryID, tokens: tokens, length: len(tokens)}
	idx.docs[entryID] = doc

	// 更新文档频率（每个 token 在该文档中只计一次）
	seenTokens := make(map[string]bool)
	termFreq := make(map[string]int)
	for _, t := range tokens {
		termFreq[t]++
		seenTokens[t] = true
	}

	for t := range seenTokens {
		idx.docFreq[t]++
		if idx.inverted[t] == nil {
			idx.inverted[t] = make(map[string]int)
		}
		idx.inverted[t][entryID] = termFreq[t]
	}

	// 更新统计
	idx.docCount++
	idx.avgDL += (float64(doc.length) - idx.avgDL) / float64(idx.docCount)
}

// Remove 从索引中删除一条文档。
func (idx *BM25Index) Remove(entryID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	doc, ok := idx.docs[entryID]
	if !ok {
		return
	}

	seenTokens := make(map[string]bool)
	for _, t := range doc.tokens {
		seenTokens[t] = true
	}

	for t := range seenTokens {
		idx.docFreq[t]--
		if idx.docFreq[t] <= 0 {
			delete(idx.docFreq, t)
			delete(idx.inverted, t)
		} else {
			delete(idx.inverted[t], entryID)
		}
	}

	delete(idx.docs, entryID)
	idx.docCount--
	if idx.docCount > 0 {
		idx.avgDL -= (float64(doc.length) - idx.avgDL) / float64(idx.docCount)
	} else {
		idx.avgDL = 0
	}
}

// Rebuild 全量重建索引。
func (idx *BM25Index) Rebuild(entries []MemoryEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.docs = make(map[string]*bm25Doc, len(entries))
	idx.inverted = make(map[string]map[string]int)
	idx.docFreq = make(map[string]int)
	idx.docCount = 0
	idx.avgDL = 0

	for _, e := range entries {
		tokens := bm25Tokenize(e.Content)
		if len(tokens) == 0 {
			continue
		}
		doc := &bm25Doc{id: e.ID, tokens: tokens, length: len(tokens)}
		idx.docs[e.ID] = doc

		seenTokens := make(map[string]bool)
		termFreq := make(map[string]int)
		for _, t := range tokens {
			termFreq[t]++
			seenTokens[t] = true
		}
		for t := range seenTokens {
			idx.docFreq[t]++
			if idx.inverted[t] == nil {
				idx.inverted[t] = make(map[string]int)
			}
			idx.inverted[t][e.ID] = termFreq[t]
		}

		idx.docCount++
		idx.avgDL += (float64(doc.length) - idx.avgDL) / float64(idx.docCount)
	}
}

// Search 执行 BM25 检索，返回按评分降序的文档 ID 和评分。
func (idx *BM25Index) Search(query string, topK int) []BM25Scored {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.docCount == 0 {
		return nil
	}

	queryTokens := bm25Tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// 计算每个匹配文档的 BM25 评分
	scores := make(map[string]float64)
	for _, qt := range queryTokens {
		postings, ok := idx.inverted[qt]
		if !ok {
			continue
		}
		idf := idx.idf(qt)
		for docID, tf := range postings {
			doc := idx.docs[docID]
			if doc == nil {
				continue
			}
			scores[docID] += idf * idx.tfSat(tf, doc.length)
		}
	}

	// 转换为排序列表
	results := make([]BM25Scored, 0, len(scores))
	for docID, score := range scores {
		results = append(results, BM25Scored{EntryID: docID, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	return results
}

// Size 返回索引中的文档数量。
func (idx *BM25Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.docCount
}

// --- 内部评分函数 ---

// idf 计算逆文档频率。
func (idx *BM25Index) idf(term string) float64 {
	df := idx.docFreq[term]
	if df <= 0 {
		return 0
	}
	n := float64(idx.docCount)
	return math.Log(1.0 + (n-float64(df)+0.5)/(float64(df)+0.5))
}

// tfSat 计算 BM25 饱和词频。
func (idx *BM25Index) tfSat(tf, docLen int) float64 {
	k1 := idx.cfg.K1
	b := idx.cfg.B
	dl := float64(docLen)
	avgdl := idx.avgDL

	if avgdl <= 0 {
		avgdl = 1
	}

	numerator := float64(tf) * (k1 + 1)
	denominator := float64(tf) + k1*(1-b+b*dl/avgdl)
	return numerator / denominator
}

// --- 中文感知分词 ---

// bm25Tokenize 对文本进行中文感知分词。
// 中文：双字组（bigram）+ 单字
// 拉丁/数字：按空格和标点拆分
// 双字组比单字更具语义区分度，能显著提升中文检索精度。
func bm25Tokenize(text string) []string {
	var tokens []string
	runes := []rune(strings.ToLower(text)) // 预转小写

	i := 0
	for i < len(runes) {
		r := runes[i]

		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			// CJK 字符：单字 token
			tokens = append(tokens, string(r))
			// 双字组（bigram）：更好的语义区分度
			if i+1 < len(runes) {
				next := runes[i+1]
				if unicode.Is(unicode.Han, next) || unicode.Is(unicode.Hiragana, next) ||
					unicode.Is(unicode.Katakana, next) || unicode.Is(unicode.Hangul, next) {
					tokens = append(tokens, string(r)+string(next))
				}
			}
			i++

		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// 拉丁/数字词：贪婪收集连续字符
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])) {
				i++
			}
			word := string(runes[start:i])
			if len(word) > 0 {
				tokens = append(tokens, word)
			}

		default:
			// 标点和空格：跳过
			i++
		}
	}

	return tokens
}

// BM25Scored 是 BM25 检索的单条结果。
type BM25Scored struct {
	EntryID string
	Score   float64
}
