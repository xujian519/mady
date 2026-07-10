package retrieval

import "strings"

// Chunk represents a segment of a larger document, with positional metadata
// for citation and context reconstruction.
type Chunk struct {
	ID        string            // unique chunk identifier
	DocID     string            // parent document identifier
	Content   string            // chunk text content
	Position  int               // chunk index within document (0-based)
	StartLine int               // starting line number in source document
	EndLine   int               // ending line number in source document
	Metadata  map[string]string // domain-specific tags (e.g., "section", "claim")
}

// ChunkOptions configures document splitting behavior.
type ChunkOptions struct {
	MaxChars       int  // maximum characters per chunk (default: 2000)
	OverlapChars   int  // character overlap between adjacent chunks (default: 200)
	SplitBySection bool // prefer splitting at section/heading boundaries
}

// DefaultChunkOptions returns sensible defaults.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		MaxChars:       2000,
		OverlapChars:   200,
		SplitBySection: true,
	}
}

// ChunkDocument splits a document into overlapping chunks.
// When SplitBySection is true, it prefers splitting at markdown heading
// boundaries (##, ###) and paragraph breaks before falling back to
// character-based splitting.
func ChunkDocument(docID, text string, opts ChunkOptions) []Chunk {
	if opts.MaxChars <= 0 {
		opts = DefaultChunkOptions()
	}

	var chunks []Chunk
	paragraphs := splitParagraphs(text)
	var current strings.Builder
	startLine := 1
	chunkIdx := 0

	flush := func() {
		if current.Len() == 0 {
			return
		}
		endLine := startLine + strings.Count(current.String(), "\n")
		chunks = append(chunks, Chunk{
			ID:        chunkID(docID, chunkIdx),
			DocID:     docID,
			Content:   current.String(),
			Position:  chunkIdx,
			StartLine: startLine,
			EndLine:   endLine,
		})
		// Advance startLine for the next chunk.
		startLine = endLine + 1
		// Overlap: keep last OverlapChars characters
		if opts.OverlapChars > 0 && current.Len() > opts.OverlapChars {
			tail := current.String()
			tailLen := len(tail)
			tail = tail[max(0, tailLen-opts.OverlapChars):]
			startLine -= strings.Count(tail, "\n")
			current.Reset()
			current.WriteString(tail)
		} else {
			current.Reset()
		}
		chunkIdx++
	}

	flushPara := func(para string) {
		// If the paragraph fits, add it normally.
		if current.Len()+len(para) <= opts.MaxChars || current.Len() == 0 {
			current.WriteString(para)
			return
		}
		// Paragraph too large for current chunk: flush and start fresh.
		if current.Len() > 0 {
			flush()
		}
		// If the paragraph itself exceeds MaxChars, split it by character.
		if len(para) > opts.MaxChars {
			for len(para) > 0 {
				cut := opts.MaxChars
				if cut > len(para) {
					cut = len(para)
				}
				current.WriteString(para[:cut])
				flush()
				para = para[cut:]
			}
		} else {
			current.WriteString(para)
		}
	}

	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// Section boundary: flush current chunk if SplitBySection is enabled.
		if opts.SplitBySection && strings.HasPrefix(para, "#") && current.Len() > 0 {
			flush()
		}

		flushPara(para)
		if i < len(paragraphs)-1 {
			current.WriteString("\n\n")
		}
	}
	flush()

	return chunks
}

// splitParagraphs splits text at paragraph boundaries (double newlines).
func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n\n")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// chunkID generates a stable chunk identifier.
func chunkID(docID string, idx int) string {
	return docID + "#chunk-" + itoa(idx)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
