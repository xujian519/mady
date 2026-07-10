package loader

import (
	"io"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// LegalStatute holds structured legal statute data.
type LegalStatute struct {
	Title     string   // statute title (e.g. "中华人民共和国民法典·合同编")
	DocID     string   // unique document identifier
	LawSource string   // legal hierarchy source (e.g. "民法典", "司法解释")
	Articles  []string // referenced article numbers (e.g. ["464", "465", "509"])
}

// LoadLegalStatute reads legal statute text from an io.Reader and loads it
// into the knowledge store. The reader is expected to provide plain text
// statute content.
//
// Usage:
//
//	loader.LoadLegalStatute(store, loader.LegalStatute{
//	    Title:     "民法典·合同编",
//	    DocID:     "civil-code-contracts",
//	    LawSource: "民法典",
//	    Articles:  []string{"464", "465", "509", "563", "577"},
//	}, strings.NewReader(statuteText))
func LoadLegalStatute(store *knowledge.Store, info LegalStatute, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	content := string(data)

	// Build structured content with metadata header.
	var b strings.Builder
	b.WriteString("法规名称: " + info.Title + "\n")
	b.WriteString("法律渊源: " + info.LawSource + "\n")
	if len(info.Articles) > 0 {
		b.WriteString("涉及法条: " + strings.Join(info.Articles, "、") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(content)

	return store.LoadLegalStatute(info.DocID, info.Title, b.String(), info.LawSource, info.Articles)
}

// LegalHierarchy defines the precedence of legal sources in Chinese law.
// Higher values indicate higher authority.
var LegalHierarchy = map[string]int{
	"宪法":         100,
	"法律":         90,
	"行政法规":    80,
	"司法解释":    70,
	"部门规章":    60,
	"地方性法规":  50,
	"指导性案例":  40,
}

// SourceRank returns the hierarchy rank of a legal source. Higher is more
// authoritative. Returns 0 for unknown sources.
func SourceRank(source string) int {
	if rank, ok := LegalHierarchy[source]; ok {
		return rank
	}
	return 0
}
