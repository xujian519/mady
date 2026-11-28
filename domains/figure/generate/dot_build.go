package generate

import (
	"fmt"
	"strings"
)

// BuildFlowchart 构建流程图 DOT。
// 决策节点（diamond）的每个后继必须带非空边标签。
func BuildFlowchart(steps []Step, opts Options) (string, error) {
	if len(steps) == 0 {
		return "", newError(ErrEmptyInput, "流程图至少需要一个步骤")
	}
	ids, known, err := indexSteps(steps)
	if err != nil {
		return "", err
	}
	assignments, err := assignFor(ids, opts)
	if err != nil {
		return "", err
	}
	lines, err := buildHeader("Flowchart", "TB", fontOf(opts), false, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for i, step := range steps {
		id := ids[i]
		shape := step.Shape
		if shape == "" {
			shape = "box"
		}
		if !flowchartShapes[shape] {
			return "", newError(ErrInvalidShape, "未知节点形状：%s", shape)
		}
		label := EscapeLabel(step.Label)
		if opts.embedNumerals() {
			label = assignments[id] + ". " + label
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s", shape=%s];`, id, label, shape))
	}
	lines = append(lines, "")
	for i, step := range steps {
		id := ids[i]
		isDecision := step.Shape == "diamond"
		for _, next := range step.Next {
			target, err := SanitizeID(next.ID)
			if err != nil {
				return "", err
			}
			if !known[target] {
				return "", newError(ErrUnknownID, "步骤 %q 的后继不存在：%s", id, target)
			}
			label := strings.TrimSpace(next.Label)
			if label == "" {
				if isDecision {
					return "", newError(ErrMissingEdgeLabel,
						"决策节点 %q 的分支必须带边标签（next：%s）", id, target)
				}
				lines = append(lines, fmt.Sprintf(`    "%s" -> "%s";`, id, target))
				continue
			}
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s" [label="%s"];`, id, target, EscapeLabel(label)))
		}
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}

// 框图的块类型 → 形状 / 填充色。
var blockShapes = map[string]string{
	"input": "invhouse", "output": "house", "process": "box",
	"storage": "cylinder", "decision": "diamond", "default": "box",
}

var blockFillColors = map[string]string{
	"input": "lightblue", "output": "lightgreen", "process": "lightyellow",
	"storage": "lightgray", "decision": "lightcoral", "default": "white",
}

// graphEdge 是框图连接与状态转移的共用形态。
type graphEdge struct{ from, to, label string }

// appendEdges 追加边；端点必须已存在，空标签输出无边标签的边。
func appendEdges(lines []string, edges []graphEdge, known map[string]bool) ([]string, error) {
	for _, edge := range edges {
		from, err := SanitizeID(edge.from)
		if err != nil {
			return nil, err
		}
		to, err := SanitizeID(edge.to)
		if err != nil {
			return nil, err
		}
		if !known[from] {
			return nil, newError(ErrUnknownID, "边的起点不存在：%s", edge.from)
		}
		if !known[to] {
			return nil, newError(ErrUnknownID, "边的终点不存在：%s", edge.to)
		}
		if label := strings.TrimSpace(edge.label); label != "" {
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s" [label="%s"];`, from, to, EscapeLabel(label)))
			continue
		}
		lines = append(lines, fmt.Sprintf(`    "%s" -> "%s";`, from, to))
	}
	return lines, nil
}

// BuildBlockDiagram 构建系统框图 DOT。
func BuildBlockDiagram(blocks []Block, connections []Connection, opts Options) (string, error) {
	if len(blocks) == 0 {
		return "", newError(ErrEmptyInput, "框图至少需要一个块")
	}
	ids := make([]string, len(blocks))
	known := make(map[string]bool, len(blocks))
	for i, block := range blocks {
		id, err := SanitizeID(block.ID)
		if err != nil {
			return "", err
		}
		if known[id] {
			return "", newError(ErrUnknownID, "块 id 重复：%s", id)
		}
		ids[i] = id
		known[id] = true
	}
	assignments, err := assignFor(ids, opts)
	if err != nil {
		return "", err
	}
	semantic := opts.Style == StyleSemantic
	lines, err := buildHeader("BlockDiagram", "LR", fontOf(opts), semantic, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for i, block := range blocks {
		id := ids[i]
		kind := block.Type
		if kind == "" {
			kind = "default"
		}
		shape, ok := blockShapes[kind]
		if !ok {
			return "", newError(ErrInvalidShape, "未知块类型：%s", kind)
		}
		label := EscapeLabel(block.Label)
		if opts.embedNumerals() {
			label = label + " (" + assignments[id] + ")"
		}
		stylePart := ", shape=" + shape
		if semantic {
			stylePart += ", fillcolor=" + blockFillColors[kind]
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s"%s];`, id, label, stylePart))
	}
	lines = append(lines, "")
	edges := make([]graphEdge, len(connections))
	for i, conn := range connections {
		edges[i] = graphEdge{from: conn.From, to: conn.To, label: conn.Label}
	}
	lines, err = appendEdges(lines, edges, known)
	if err != nil {
		return "", err
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}

// BuildHierarchy 构建组件层级图 DOT（先序展平，标号按出现顺序分配）。
func BuildHierarchy(tree []HierarchyNode, opts Options) (string, error) {
	if len(tree) == 0 {
		return "", newError(ErrEmptyInput, "层级图至少需要一个根节点")
	}
	var ids []string
	labels := make(map[string]string)
	parents := make(map[string]string)
	seen := make(map[string]bool)
	var visit func(node HierarchyNode, parent string) error
	visit = func(node HierarchyNode, parent string) error {
		id, err := SanitizeID(node.ID)
		if err != nil {
			return err
		}
		if seen[id] {
			return newError(ErrUnknownID, "节点 id 重复：%s", node.ID)
		}
		seen[id] = true
		ids = append(ids, id)
		labels[id] = node.Label
		if parent != "" {
			parents[id] = parent
		}
		for _, child := range node.Children {
			if err := visit(child, id); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range tree {
		if err := visit(root, ""); err != nil {
			return "", err
		}
	}
	assignments, err := assignFor(ids, opts)
	if err != nil {
		return "", err
	}
	lines, err := buildHeader("ComponentHierarchy", "TB", fontOf(opts), false, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		label := EscapeLabel(labels[id])
		if opts.embedNumerals() {
			label = label + " (" + assignments[id] + ")"
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s"];`, id, label))
	}
	lines = append(lines, "")
	for _, id := range ids {
		if parent, ok := parents[id]; ok {
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s";`, parent, id))
		}
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}

// BuildStateDiagram 构建状态转移图 DOT。initial 伪状态画实心小圆且不分配标号。
func BuildStateDiagram(states []State, transitions []Transition, opts Options) (string, error) {
	if len(states) == 0 {
		return "", newError(ErrEmptyInput, "状态图至少需要一个状态")
	}
	known := make(map[string]bool, len(states))
	var labeled []string
	for _, state := range states {
		id, err := SanitizeID(state.ID)
		if err != nil {
			return "", err
		}
		if known[id] {
			return "", newError(ErrUnknownID, "状态 id 重复：%s", id)
		}
		known[id] = true
		if strings.TrimSpace(state.Label) != "" {
			labeled = append(labeled, id)
		}
	}
	assignments, err := assignFor(labeled, opts)
	if err != nil {
		return "", err
	}
	lines, err := buildHeader("StateDiagram", "TB", fontOf(opts), false, opts.Page, opts.FontPt)
	if err != nil {
		return "", err
	}
	for _, state := range states {
		id, _ := SanitizeID(state.ID)
		kind := state.Kind
		if kind == "initial" {
			lines = append(lines,
				fmt.Sprintf(`    "%s" [label="", shape=circle, style=filled, fillcolor=black, width=0.25, fixedsize=true];`, id))
			continue
		}
		label := EscapeLabel(state.Label)
		if numeral, ok := assignments[id]; ok && opts.embedNumerals() {
			label = numeral + ". " + label
		}
		if kind == "final" {
			lines = append(lines, fmt.Sprintf(`    "%s" [label="%s", shape=doublecircle];`, id, label))
			continue
		}
		lines = append(lines, fmt.Sprintf(`    "%s" [label="%s", shape=box, style=rounded];`, id, label))
	}
	lines = append(lines, "")
	edges := make([]graphEdge, len(transitions))
	for i, transition := range transitions {
		edges[i] = graphEdge{from: transition.From, to: transition.To, label: transition.Label}
	}
	lines, err = appendEdges(lines, edges, known)
	if err != nil {
		return "", err
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n"), nil
}
