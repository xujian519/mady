package literature

import "context"

// Paper 是一条归一化的文献命中。
type Paper struct {
	ID       string   `json:"id"`
	DOI      string   `json:"doi,omitempty"`
	Source   string   `json:"source"`
	Title    string   `json:"title"`
	Authors  []string `json:"authors,omitempty"`
	Year     int      `json:"year,omitempty"`
	Venue    string   `json:"venue,omitempty"`
	Abstract string   `json:"abstract,omitempty"`
	URL      string   `json:"url,omitempty"`
	PDFURL   string   `json:"pdf_url,omitempty"`
	CitedBy  int      `json:"cited_by,omitempty"`
}

// SearchOptions 是检索参数。
type SearchOptions struct {
	Limit    int
	YearFrom int
}

// Connector 是一个文献源。
type Connector interface {
	// ID 是稳定标识（openalex / semantic-scholar / arxiv / crossref）。
	ID() string
	// Name 是展示名。
	Name() string
	Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error)
	// Fetch 按源内 ID 取单条记录；源不支持时返回 nil。
	Fetch(ctx context.Context, id string) (*Paper, error)
}

// SourceInfo 是源清单条目。
type SourceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}
