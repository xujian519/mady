package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// NewPatentKGQueryTool creates the patent_kg_query tool backed by the given
// GraphStore. The tool supports three query modes:
//   - query: keyword search over node names/titles/content
//   - id:    expand a node and its semantic neighbors
//   - node_type: list nodes of a given type (with Sati aliases like Judgment/LawArticle)
func NewPatentKGQueryTool(store *GraphStore) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "patent_kg_query",
		Description: "查询专利知识图谱节点（判例/审查规则/法条/概念）。三种模式：query 关键词检索、id 节点展开详情与邻居、node_type 按类型浏览。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":           map[string]any{"type": "string", "description": "关键词检索（如 创造性 三步法）；与 id 二选一"},
				"id":              map[string]any{"type": "string", "description": "节点 id；与 query 二选一，id 优先"},
				"node_type":       map[string]any{"type": "string", "description": "按节点类型浏览（Case/Judgment/LawArticle/GuidelineRule/Clause/WikiCard/Concept）"},
				"expand":          map[string]any{"type": "boolean", "default": true, "description": "关键词命中后是否做关系扩展"},
				"include_content": map[string]any{"type": "boolean", "default": false, "description": "是否附节点正文片段"},
				"limit":           map[string]any{"type": "integer", "default": 5, "description": "返回条数上限（默认 5，最大 10）"},
			},
		},
		ReadOnly: true,
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Query          string `json:"query"`
				ID             string `json:"id"`
				NodeType       string `json:"node_type"`
				Expand         bool   `json:"expand"`
				IncludeContent bool   `json:"include_content"`
				Limit          int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "知识图谱查询参数格式错误"), nil
			}
			limit := p.Limit
			if limit <= 0 || limit > 10 {
				limit = 5
			}
			if store == nil {
				return agentcore.NewFailureResult("知识图谱未加载", "当前未配置专利知识图谱"), nil
			}

			var hits []patentKGHit
			switch {
			case p.ID != "":
				hits = queryKGByID(store, p.ID, limit, p.IncludeContent)
			case p.Query != "":
				hits = queryKGByKeyword(store, p.Query, limit, p.Expand, p.IncludeContent)
			case p.NodeType != "":
				hits = queryKGByType(store, p.NodeType, limit, p.IncludeContent)
			default:
				return "请提供 query、id 或 node_type 之一", nil
			}
			return formatKGQueryResult(hits), nil
		},
	}
}

type patentKGHit struct {
	ID        string             `json:"id"`
	NodeType  string             `json:"node_type"`
	Name      string             `json:"name,omitempty"`
	Title     string             `json:"title,omitempty"`
	Via       string             `json:"via,omitempty"`
	Relation  string             `json:"relation,omitempty"`
	Content   string             `json:"content,omitempty"`
	Neighbors []patentKGNeighbor `json:"neighbors,omitempty"`
}

type patentKGNeighbor struct {
	ID       string `json:"id"`
	NodeType string `json:"node_type"`
	Name     string `json:"name,omitempty"`
	Title    string `json:"title,omitempty"`
	Relation string `json:"relation"`
}

func queryKGByID(store *GraphStore, id string, limit int, includeContent bool) []patentKGHit {
	node := store.GetNode(id)
	if node == nil {
		return nil
	}
	hit := nodeToKGHit(node, includeContent)
	for _, edge := range store.GetOutgoing(id) {
		if isWeakRelation(edge.Relation) {
			continue
		}
		if len(hit.Neighbors) >= limit {
			break
		}
		if n := store.GetNode(edge.TargetID); n != nil {
			hit.Neighbors = append(hit.Neighbors, patentKGNeighbor{
				ID:       n.ID,
				NodeType: n.NodeType,
				Name:     n.Name,
				Title:    n.Title,
				Relation: edge.Relation,
			})
		}
	}
	return []patentKGHit{hit}
}

func queryKGByKeyword(store *GraphStore, query string, limit int, expand, includeContent bool) []patentKGHit {
	_ = expand // Reserved for future relation expansion; currently uses substring search.
	kw := strings.ToLower(query)
	matches := store.SearchGraphNodes(kw, "", limit*3)
	if len(matches) < limit && len([]rune(kw)) <= 4 {
		all := store.AllNodes()
		seen := make(map[string]bool)
		for _, m := range matches {
			seen[m.ID] = true
		}
		for _, n := range all {
			if seen[n.ID] {
				continue
			}
			if strings.Contains(strings.ToLower(n.Name), kw) ||
				strings.Contains(strings.ToLower(n.Title), kw) ||
				strings.Contains(strings.ToLower(n.Content), kw) {
				matches = append(matches, n)
				seen[n.ID] = true
				if len(matches) >= limit*3 {
					break
				}
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].AuthorityWeight != matches[j].AuthorityWeight {
			return matches[i].AuthorityWeight > matches[j].AuthorityWeight
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}

	hits := make([]patentKGHit, len(matches))
	for i, n := range matches {
		hits[i] = nodeToKGHit(n, includeContent)
	}
	return hits
}

func queryKGByType(store *GraphStore, nodeType string, limit int, includeContent bool) []patentKGHit {
	resolved := resolveKGNodeTypeAlias(nodeType)
	nodes := store.SearchGraphNodes("", resolved, limit)
	hits := make([]patentKGHit, len(nodes))
	for i, n := range nodes {
		hits[i] = nodeToKGHit(n, includeContent)
	}
	return hits
}

func resolveKGNodeTypeAlias(t string) string {
	switch strings.ToLower(t) {
	case "judgment":
		return NodeJudgment
	case "lawarticle", "law_article":
		return NodeLawArticle
	}
	return t
}

func nodeToKGHit(n *GraphNode, includeContent bool) patentKGHit {
	h := patentKGHit{
		ID:       n.ID,
		NodeType: n.NodeType,
		Name:     n.Name,
		Title:    n.Title,
	}
	if includeContent {
		h.Content = truncateKGContent(n.Content, 600)
	}
	return h
}

func formatKGQueryResult(hits []patentKGHit) any {
	if len(hits) == 0 {
		return "未找到知识图谱节点"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 个节点:\n", len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "\n[%d] %s [%s] (id: %s)\n", i+1, h.Name, h.NodeType, h.ID)
		if h.Title != "" && h.Title != h.Name {
			fmt.Fprintf(&b, "标题: %s\n", h.Title)
		}
		if h.Content != "" {
			fmt.Fprintf(&b, "%s\n", h.Content)
		}
		for _, nb := range h.Neighbors {
			fmt.Fprintf(&b, "  → %s [%s] via %s\n", nb.Name, nb.NodeType, nb.Relation)
		}
	}
	return b.String()
}

func truncateKGContent(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	return string(r[:maxChars]) + fmt.Sprintf("\n…（截断，共 %d 字符）", len(r))
}
