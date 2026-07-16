package knowledge

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/xujian519/mady/pkg/csync"
	"github.com/xujian519/mady/retrieval"
)

// Store manages document collections organized by domain.
// It handles loading, chunking, indexing, and provides retrieval hooks
// for Agent integration.
type Store struct {
	docs     *csync.Map[string, *Document]         // docID → Document
	chunks   *csync.Map[string, []retrieval.Chunk] // docID → chunks
	byDomain *csync.Map[string, []string]          // domain → []docID

	chunkOpts retrieval.ChunkOptions
}

// Document represents a loaded knowledge document.
type Document struct {
	ID         string
	Title      string
	Domain     string
	Content    string
	Source     string            // file path, URL, or "inline"
	Metadata   map[string]string // domain-specific metadata (e.g. "ipc": "G06F17/30", "law": "民法典")
	Searchable bool              // false for index/directory pages that should be excluded from retrieval
}

// NewStore creates a new knowledge store.
func NewStore() *Store {
	return &Store{
		docs:      csync.NewMap[string, *Document](),
		chunks:    csync.NewMap[string, []retrieval.Chunk](),
		byDomain:  csync.NewMap[string, []string](),
		chunkOpts: retrieval.DefaultChunkOptions(),
	}
}

// LoadDocument loads a document from a file path into the store.
func (s *Store) LoadDocument(domain, docID, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("knowledge: load %s: %w", docID, err)
	}
	return s.AddDocument(domain, docID, filePath, string(data), filePath)
}

// LoadText loads inline text as a document.
func (s *Store) LoadText(domain, docID, title, content string) error {
	return s.AddDocument(domain, docID, title, content, "inline")
}

// AddDocument adds a document to the store and chunks it for retrieval.
func (s *Store) AddDocument(domain, docID, title, content, source string) error {
	doc := &Document{
		ID:         docID,
		Searchable: true, // default: searchable unless explicitly marked otherwise
		Title:      title,
		Domain:     domain,
		Content:    content,
		Source:     source,
	}
	s.docs.Set(docID, doc)

	// Chunk the document for retrieval.
	chunks := retrieval.ChunkDocument(docID, content, s.chunkOpts)
	s.chunks.Set(docID, chunks)

	domainIDs, _ := s.byDomain.Get(domain)
	domainIDs = append(domainIDs, docID)
	s.byDomain.Set(domain, domainIDs)
	return nil
}

// GetDocument returns a document by ID.
func (s *Store) GetDocument(docID string) (*Document, bool) {
	return s.docs.Get(docID)
}

// AllDocIDs returns all document IDs in the store.
func (s *Store) AllDocIDs() []string {
	docs := s.docs.Copy()
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	return ids
}

// SearchableDocCount returns the count of searchable documents.
func (s *Store) SearchableDocCount() int {
	docs := s.docs.Copy()
	count := 0
	for _, doc := range docs {
		if doc.Searchable {
			count++
		}
	}
	return count
}

// ChunksForDomain returns all chunks for documents in a given domain.
func (s *Store) ChunksForDomain(domain string) []retrieval.Chunk {
	docIDs, _ := s.byDomain.Get(domain)

	var all []retrieval.Chunk
	for _, docID := range docIDs {
		chunks, _ := s.chunks.Get(docID)
		all = append(all, chunks...)
	}
	return all
}

// SearchableChunksForDomain returns chunks only from documents marked Searchable.
// Directory/index pages (Searchable=false) are excluded. This is the preferred
// method for RAG retrieval to avoid noise from index/navigation pages.
func (s *Store) SearchableChunksForDomain(domain string) []retrieval.Chunk {
	docIDs, _ := s.byDomain.Get(domain)

	var all []retrieval.Chunk
	for _, docID := range docIDs {
		doc, ok := s.docs.Get(docID)
		if !ok || !doc.Searchable {
			continue
		}
		chunks, _ := s.chunks.Get(docID)
		all = append(all, chunks...)
	}
	return all
}

// AllChunks returns all chunks across all domains.
func (s *Store) AllChunks() []retrieval.Chunk {
	chunks := s.chunks.Copy()
	var all []retrieval.Chunk
	for _, chunkList := range chunks {
		all = append(all, chunkList...)
	}
	return all
}

// RetrievalHook creates an Agent retrieval hook scoped to a domain.
// It uses SearchableChunksForDomain to exclude directory/index pages from retrieval.
// The hook automatically searches the domain's document chunks on each
// model call and injects relevant context.
func (s *Store) RetrievalHook(domain string, config retrieval.RetrievalConfig) *retrieval.RetrievalHook {
	chunks := s.SearchableChunksForDomain(domain)
	if len(chunks) == 0 {
		chunks = s.AllChunks()
	}
	if config.DomainHint == "" {
		config.DomainHint = domain
	}
	return retrieval.NewRetrievalHook(chunks, config)
}

// Stats returns store statistics.
func (s *Store) Stats() StoreStats {
	docs := s.docs.Copy()
	chunks := s.chunks.Copy()
	byDomain := s.byDomain.Copy()

	stats := StoreStats{
		TotalDocs:   len(docs),
		TotalChunks: 0,
		ByDomain:    make(map[string]int),
	}
	for _, chunkList := range chunks {
		stats.TotalChunks += len(chunkList)
	}
	for domain, ids := range byDomain {
		stats.ByDomain[domain] = len(ids)
	}
	return stats
}

// StoreStats holds aggregate knowledge store statistics.
type StoreStats struct {
	TotalDocs   int
	TotalChunks int
	ByDomain    map[string]int
}

// String formats stats for display.
func (s StoreStats) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "文档: %d, 分块: %d", s.TotalDocs, s.TotalChunks)
	for domain, count := range s.ByDomain {
		fmt.Fprintf(&b, "\n  %s: %d 篇", domain, count)
	}
	return b.String()
}

// ReindexVectors computes embeddings for all searchable chunks in the store
// and stores them in chunk metadata. This is a potentially expensive operation
// that calls the embedding API for each chunk. Call it after bulk imports.
//
// The embedder is used to compute vectors; chunks already having an embedding
// are skipped to avoid redundant API calls.
func (s *Store) ReindexVectors(ctx context.Context, embedder retrieval.Embedder) error {
	if embedder == nil {
		return fmt.Errorf("knowledge: embedder is nil")
	}

	type chunkRef struct {
		docID   string
		chunkID string
		content string
	}
	var pending []chunkRef
	chunks := s.chunks.Copy()
	docs := s.docs.Copy()
	for docID, docChunks := range chunks {
		doc := docs[docID]
		if doc != nil && !doc.Searchable {
			continue
		}
		for i := range docChunks {
			if _, hasEmbedding := docChunks[i].Metadata["embedding"]; hasEmbedding {
				continue
			}
			pending = append(pending, chunkRef{
				docID:   docID,
				chunkID: docChunks[i].ID,
				content: docChunks[i].Content,
			})
		}
	}

	if len(pending) == 0 {
		return nil
	}

	type batchResult struct {
		refs []chunkRef
		vecs [][]float32
	}

	batchSize := 100
	for i := 0; i < len(pending); i += batchSize {
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[i:end]
		texts := make([]string, len(batch))
		for j, ref := range batch {
			texts[j] = ref.content
		}

		vectors, err := embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("knowledge: embed batch %d-%d: %w", i, end, err)
		}

		result := batchResult{refs: batch, vecs: vectors}

		for j, vec := range result.vecs {
			curChunks, ok := s.chunks.Get(result.refs[j].docID)
			if !ok {
				continue
			}
			// Deep copy to avoid sharing metadata map with csync.Map value.
			newChunks := make([]retrieval.Chunk, len(curChunks))
			copy(newChunks, curChunks)
			for k := range newChunks {
				if newChunks[k].ID == result.refs[j].chunkID {
					meta := make(map[string]string, len(newChunks[k].Metadata)+1)
					for mk, mv := range newChunks[k].Metadata {
						meta[mk] = mv
					}
					newChunks[k].Metadata = meta
					retrieval.StoreEmbedding(&newChunks[k], vec)
					break
				}
			}
			s.chunks.Set(result.refs[j].docID, newChunks)
		}
	}

	return nil
}

// LoadPatentClaims loads patent claim documents with IPC metadata.
// The content is structured as claims text with embedded IPC classification.
func (s *Store) LoadPatentClaims(docID, title, content string, ipc string) error {
	doc := &Document{
		ID:      docID,
		Title:   title,
		Domain:  "patent",
		Content: content,
		Source:  "inline",
		Metadata: map[string]string{
			"ipc":  ipc,
			"type": "claims",
		},
	}
	s.docs.Set(docID, doc)

	chunks := retrieval.ChunkDocument(docID, content, s.chunkOpts)
	// Tag chunks with metadata for retrieval.
	for i := range chunks {
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]string)
		}
		chunks[i].Metadata["ipc"] = ipc
		chunks[i].Metadata["type"] = "claims"
	}
	s.chunks.Set(docID, chunks)
	domainIDs, _ := s.byDomain.Get("patent")
	domainIDs = append(domainIDs, docID)
	s.byDomain.Set("patent", domainIDs)
	return nil
}

// LoadLegalStatute loads a legal statute document with law metadata.
func (s *Store) LoadLegalStatute(docID, title, content string, lawSource string, articles []string) error {
	doc := &Document{
		ID:      docID,
		Title:   title,
		Domain:  "legal",
		Content: content,
		Source:  "inline",
		Metadata: map[string]string{
			"law_source": lawSource,
			"type":       "statute",
		},
	}
	s.docs.Set(docID, doc)

	chunks := retrieval.ChunkDocument(docID, content, s.chunkOpts)
	for i := range chunks {
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]string)
		}
		chunks[i].Metadata["law_source"] = lawSource
		chunks[i].Metadata["type"] = "statute"
	}
	s.chunks.Set(docID, chunks)
	domainIDs, _ := s.byDomain.Get("legal")
	domainIDs = append(domainIDs, docID)
	s.byDomain.Set("legal", domainIDs)
	return nil
}

// SeedData loads embedded seed documents for quick start without external files.
// Returns the number of documents loaded.
func (s *Store) SeedData() int {
	count := 0
	for _, sd := range seedDocuments {
		if err := s.AddDocument(sd.Domain, sd.ID, sd.Title, sd.Content, "embedded"); err != nil {
			continue
		}
		count++
	}
	return count
}

// seedDocument is an embedded knowledge document used for seeding.
type seedDocument struct {
	ID      string
	Title   string
	Domain  string
	Content string
}

// seedDocuments provides embedded seed knowledge for patent and legal domains.
// These are concise examples demonstrating the document structure; production
// deployments should load real patent/law data from external sources.
var seedDocuments = []seedDocument{
	{
		ID:      "patent-law-cn",
		Title:   "中华人民共和国专利法（2020年修正）摘要",
		Domain:  "patent",
		Content: patentLawSeed,
	},
	{
		ID:      "civil-code-contracts",
		Title:   "中华人民共和国民法典·合同编（要点）",
		Domain:  "legal",
		Content: civilCodeContractsSeed,
	},
}

const patentLawSeed = `中华人民共和国专利法（2020年修正）
（2020年10月17日第十三届全国人民代表大会常务委员会第二十二次会议通过）

第二条 本法所称的发明创造是指发明、实用新型和外观设计。
发明，是指对产品、方法或者其改进所提出的新的技术方案。
实用新型，是指对产品的形状、构造或者其结合所提出的适于实用的新的技术方案。
外观设计，是指对产品的整体或者局部的形状、图案或者其结合以及色彩与形状、图案的结合所作出的富有美感并适于工业应用的新设计。

第二十二条 授予专利权的发明和实用新型，应当具备新颖性、创造性和实用性。
新颖性，是指该发明或者实用新型不属于现有技术；也没有任何单位或者个人就同样的发明或者实用新型在申请日以前向国务院专利行政部门提出过申请，并记载在申请日以后公布的专利申请文件或者公告的专利文件中。
创造性，是指与现有技术相比，该发明具有突出的实质性特点和显著的进步，该实用新型具有实质性特点和进步。
实用性，是指该发明或者实用新型能够制造或者使用，并且能够产生积极效果。

第二十五条 对下列各项，不授予专利权：
（一）科学发现；
（二）智力活动的规则和方法；
（三）疾病的诊断和治疗方法；
（四）动物和植物品种；
（五）原子核变换方法以及用原子核变换方法获得的物质；
（六）对平面印刷品的图案、色彩或者二者的结合作出的主要起标识作用的设计。`

const civilCodeContractsSeed = `中华人民共和国民法典·合同编（要点）

第四百六十四条 合同是民事主体之间设立、变更、终止民事法律关系的协议。
婚姻、收养、监护等有关身份关系的协议，适用有关该身份关系的法律规定；没有规定的，可以根据其性质参照适用本编规定。

第四百六十五条 依法成立的合同，受法律保护。
依法成立的合同，仅对当事人具有法律约束力，但是法律另有规定的除外。

第五百零九条 当事人应当按照约定全面履行自己的义务。
当事人应当遵循诚信原则，根据合同的性质、目的和交易习惯履行通知、协助、保密等义务。

第五百六十三条 有下列情形之一的，当事人可以解除合同：
（一）因不可抗力致使不能实现合同目的；
（二）在履行期限届满前，当事人一方明确表示或者以自己的行为表明不履行主要债务；
（三）当事人一方迟延履行主要债务，经催告后在合理期限内仍未履行；
（四）当事人一方迟延履行债务或者有其他违约行为致使不能实现合同目的；
（五）法律规定的其他情形。

第五百七十七条 当事人一方不履行合同义务或者履行合同义务不符合约定的，应当承担继续履行、采取补救措施或者赔偿损失等违约责任。`
