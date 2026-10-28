package generate

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// 渲染复核：在已生成的 SVG 源上量测「只有看渲染结果才看得见」的问题。
//
// 本实现覆盖四类：文字被线条贯穿、文字与图线净距不足、字高低于内控下限、墨迹越出
// 画布；无法量测的结构统一记 not-measured，而不是略过——报告里没有 not-measured 时，
// 「未发现问题」才等于逐类量测过。点划线覆盖、剖面线取向碰撞、图元叠压等类别本轮
// 未实现（属 F4 的一致性/交付核对范围）。
//
// 量测对象是矢量源：元素按嵌套继承 transform、字号、text-anchor 与描边；曲线按
// 8 段折线采样；同一语义分组（<g class="node|edge">）内的文字与线段不互判——节点
// 标签本就排在节点框内、边标签本就贴着自己的边，那是排版而非缺陷。引线
// （data-figure-role="leader"）不参与净距判定：引线本来就止于文字外框。

// CheckKind 是复核发现的问题类别。
type CheckKind string

// Check kind constants.
const (
	CheckTextCrossed   CheckKind = "text-crossed-by-line"
	CheckTextClearance CheckKind = "text-clearance"
	CheckFontBelowMin  CheckKind = "font-below-minimum"
	CheckInkOutside    CheckKind = "ink-outside-canvas"
	CheckNotMeasured   CheckKind = "not-measured"
)

// CheckFinding 是一条复核发现。
type CheckFinding struct {
	Kind     CheckKind `json:"check"`
	Message  string    `json:"message"`
	Blocking bool      `json:"blocking"`
}

// CheckReport 是一张附图的复核报告。
type CheckReport struct {
	WidthMm   float64        `json:"width_mm,omitempty"`
	HeightMm  float64        `json:"height_mm,omitempty"`
	TextCount int            `json:"text_count"`
	MinFontMm float64        `json:"min_font_mm,omitempty"`
	Findings  []CheckFinding `json:"findings"`
}

// CheckOptions 是复核参数。
type CheckOptions struct {
	// TextClearanceMm 为文字与图线的净距下限（毫米）；0 关闭该判据。
	TextClearanceMm float64
	// MinFontMm 为字高内控下限（毫米）；0 关闭。
	MinFontMm float64
	// MaxBytes 为输入上限（字节）；0 用 DefaultSVGMaxBytes。
	MaxBytes int
	// FigureType 用于缺省净距分档（见 ClearanceMm）。
	FigureType string
}

// 按图型分档的净距下限（毫米）：取实测下界再减 0.1，只抓真正的贴靠与重叠。
var clearanceTiers = map[string]float64{
	"cross_section":       1.5,
	"section":             1.5,
	"structure":           1.5,
	"flowchart":           1.5,
	"state_diagram":       1.5,
	"state":               1.5,
	"block_diagram":       0.5,
	"block":               0.5,
	"component_hierarchy": 0.5,
	"hierarchy":           0.5,
	"template":            0.5,
	"sequence_diagram":    0.7,
	"sequence":            0.7,
}

// ClearanceMm 返回图型的净距档位；未登记图型返回 0（不判净距，仍判穿字）。
func ClearanceMm(figureType string) float64 {
	return clearanceTiers[figureType]
}

// BlockingFindings 返回阻断级发现（not-measured 不算）。
func BlockingFindings(report CheckReport) []CheckFinding {
	var out []CheckFinding
	for _, finding := range report.Findings {
		if finding.Blocking {
			out = append(out, finding)
		}
	}
	return out
}

// ---------- 扫描 ----------

type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

func (m matrix) apply(p [2]float64) [2]float64 {
	return [2]float64{m[0]*p[0] + m[2]*p[1] + m[4], m[1]*p[0] + m[3]*p[1] + m[5]}
}

// multiply 返回 m·n（先应用 n，再应用 m）。
func (m matrix) multiply(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m matrix) scaleFactor() float64 {
	return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
}

type scanState struct {
	matrix   matrix
	fontSize float64
	anchor   string
	groupID  int
	hasGroup bool
	leader   bool
	skip     bool
}

type scanResult struct {
	texts       []scannedText
	segments    []scannedSegment
	notMeasured []string
}

type scannedText struct {
	content  string
	box      GlyphBox
	fontSize float64
	groupID  int
	hasGroup bool
}

type scannedSegment struct {
	from, to [2]float64
	groupID  int
	hasGroup bool
	leader   bool
}

// findTagEnd 返回从 start 处的 '<' 开始的下一个标签结束位置（引号内的 '>' 不算）。
func findTagEnd(svg string, start int) (int, bool) {
	inSingle, inDouble := false, false
	for i := start + 1; i < len(svg); i++ {
		switch svg[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '>':
			if !inSingle && !inDouble {
				return i + 1, true
			}
		}
	}
	return 0, false
}

// attrValue 取标签内属性的值（属性名不区分大小写）。
func tagAttr(tag, name string) (string, bool) {
	lower := strings.ToLower(tag)
	needle := strings.ToLower(name) + "="
	index := 0
	for {
		found := strings.Index(lower[index:], needle)
		if found < 0 {
			return "", false
		}
		start := index + found + len(needle)
		// 属性名左侧必须是空白，避免匹配到 data-x 中的子串。
		if start-len(needle)-1 >= 0 {
			prev := lower[start-len(needle)-1]
			if prev != ' ' && prev != '\t' && prev != '\n' && prev != '\r' {
				index = start
				continue
			}
		}
		if start >= len(tag) {
			return "", false
		}
		quote := tag[start]
		if quote != '"' && quote != '\'' {
			return "", false
		}
		end := strings.IndexByte(tag[start+1:], quote)
		if end < 0 {
			return "", false
		}
		return tag[start+1 : start+1+end], true
	}
}

func attrFloat(tag, name string) (float64, bool) {
	value, ok := tagAttr(tag, name)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func tagName(tag string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(tag, "<"))
	trimmed = strings.TrimSuffix(trimmed, ">")
	trimmed = strings.TrimSuffix(trimmed, "/")
	end := strings.IndexAny(trimmed, " \t\r\n")
	if end >= 0 {
		trimmed = trimmed[:end]
	}
	return strings.ToLower(trimmed)
}

func isClosingTag(tag string) bool { return strings.HasPrefix(strings.TrimSpace(tag), "</") }
func isSelfClosing(tag string) bool {
	trimmed := strings.TrimSpace(tag)
	return strings.HasSuffix(trimmed, "/>")
}

// parseTransform 解析 transform 列表；遇到不支持的形式返回 false。
//
//nolint:gocognit // transform 语法分支集中在此，拆分只会把同一个数字列表在几个函数间来回传
func parseTransform(value string) (matrix, bool) {
	result := identity
	remaining := strings.TrimSpace(value)
	for remaining != "" {
		open := strings.IndexByte(remaining, '(')
		if open < 0 {
			return identity, false
		}
		name := strings.ToLower(strings.TrimSpace(remaining[:open]))
		closeIndex := strings.IndexByte(remaining[open:], ')')
		if closeIndex < 0 {
			return identity, false
		}
		args := strings.FieldsFunc(remaining[open+1:open+closeIndex], func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n'
		})
		numbers := make([]float64, len(args))
		for i, arg := range args {
			parsed, err := strconv.ParseFloat(arg, 64)
			if err != nil {
				return identity, false
			}
			numbers[i] = parsed
		}
		var step matrix
		switch name {
		case "translate":
			if len(numbers) == 1 {
				step = matrix{1, 0, 0, 1, numbers[0], 0}
			} else if len(numbers) == 2 {
				step = matrix{1, 0, 0, 1, numbers[0], numbers[1]}
			} else {
				return identity, false
			}
		case "scale":
			if len(numbers) == 1 {
				step = matrix{numbers[0], 0, 0, numbers[0], 0, 0}
			} else if len(numbers) == 2 {
				step = matrix{numbers[0], 0, 0, numbers[1], 0, 0}
			} else {
				return identity, false
			}
		case "rotate":
			if len(numbers) != 1 && len(numbers) != 3 {
				return identity, false
			}
			radians := numbers[0] * math.Pi / 180
			cosine, sine := math.Cos(radians), math.Sin(radians)
			rotation := matrix{cosine, sine, -sine, cosine, 0, 0}
			if len(numbers) == 3 {
				rotation = matrix{1, 0, 0, 1, numbers[1], numbers[2]}.
					multiply(rotation).
					multiply(matrix{1, 0, 0, 1, -numbers[1], -numbers[2]})
			}
			step = rotation
		case "matrix":
			if len(numbers) != 6 {
				return identity, false
			}
			step = matrix{numbers[0], numbers[1], numbers[2], numbers[3], numbers[4], numbers[5]}
		default:
			return identity, false
		}
		result = result.multiply(step)
		remaining = strings.TrimSpace(remaining[open+closeIndex+1:])
	}
	return result, true
}

// scanSVG 扫描根元素的全部可量测图元。
//
//nolint:gocognit // 单趟标签扫描与状态栈天然分支密集；拆分会引入共享状态在函数间传递
func scanSVG(svg string) scanResult {
	result := scanResult{}
	notMeasured := map[string]bool{}
	note := func(reason string) {
		if !notMeasured[reason] {
			notMeasured[reason] = true
			result.notMeasured = append(result.notMeasured, reason)
		}
	}

	stack := []scanState{{matrix: identity, fontSize: 0, anchor: AnchorStart}}
	current := func() *scanState { return &stack[len(stack)-1] }
	nextGroupID := 0

	position := 0
	for position < len(svg) {
		next := strings.IndexByte(svg[position:], '<')
		if next < 0 {
			break
		}
		start := position + next
		if strings.HasPrefix(svg[start:], "<!--") {
			end := strings.Index(svg[start:], "-->")
			if end < 0 {
				break
			}
			position = start + end + 3
			continue
		}
		if strings.HasPrefix(svg[start:], "<?") || strings.HasPrefix(svg[start:], "<!") {
			end := strings.IndexByte(svg[start:], '>')
			if end < 0 {
				break
			}
			position = start + end + 1
			continue
		}
		end, ok := findTagEnd(svg, start)
		if !ok {
			break
		}
		tag := svg[start:end]
		position = end

		if isClosingTag(tag) {
			name := tagName(tag)
			if name == "g" || name == "defs" || name == "clippath" || name == "marker" ||
				name == "symbol" || name == "pattern" {
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
			}
			continue
		}

		name := tagName(tag)
		selfClosing := isSelfClosing(tag)
		state := *current()
		nextState := state

		if transform, ok := tagAttr(tag, "transform"); ok {
			parsed, valid := parseTransform(transform)
			if !valid {
				note("无法解析的 transform，相关图元的量测结果不可靠")
			} else {
				nextState.matrix = state.matrix.multiply(parsed)
			}
		}
		if fontSize, ok := attrFloat(tag, "font-size"); ok {
			nextState.fontSize = fontSize
		}
		if anchor, ok := tagAttr(tag, "text-anchor"); ok {
			nextState.anchor = anchor
		}
		if role, ok := tagAttr(tag, "data-figure-role"); ok && strings.EqualFold(role, "leader") {
			nextState.leader = true
		}
		if style, ok := tagAttr(tag, "style"); ok && strings.Contains(strings.ReplaceAll(style, " ", ""), "display:none") {
			nextState.skip = true
		}

		switch name {
		case "svg":
			if len(stack) > 1 {
				note("嵌套 <svg> 的内层视口未量测")
			}
			if !selfClosing {
				stack = append(stack, nextState)
			}
		case "g":
			if class, ok := tagAttr(tag, "class"); ok {
				class = strings.ToLower(class)
				if class == "node" || class == "edge" {
					nextGroupID++
					nextState.groupID = nextGroupID
					nextState.hasGroup = true
				}
			}
			if !selfClosing {
				stack = append(stack, nextState)
			}
		case "defs", "clippath", "marker", "symbol", "pattern":
			nextState.skip = true
			if !selfClosing {
				stack = append(stack, nextState)
			}
		case "use", "image":
			note("<use>/<image> 引用的内容未量测")
		case "style":
			note("<style> 样式表未量测")
		case "text":
			if selfClosing {
				break
			}
			closeIndex := strings.Index(strings.ToLower(svg[position:]), "</text>")
			if closeIndex < 0 {
				break
			}
			inner := svg[position : position+closeIndex]
			position += closeIndex + len("</text>")
			if nextState.skip {
				break
			}
			if strings.Contains(inner, "<tspan") {
				note("<tspan> 的 x/y/dx/dy 偏移未量测")
			}
			content := decodeEntities(stripTags(inner))
			content = strings.TrimSpace(content)
			if content == "" {
				break
			}
			x, _ := attrFloat(tag, "x")
			if x == 0 {
				x, _ = attrFloat(tag, "X")
			}
			y, okY := attrFloat(tag, "y")
			if !okY {
				break
			}
			fontSize := nextState.fontSize
			if size, ok := attrFloat(tag, "font-size"); ok {
				fontSize = size
			}
			if fontSize <= 0 {
				fontSize = 16 // CSS 初值
			}
			baseline := nextState.matrix.apply([2]float64{x, y})
			scale := nextState.matrix.scaleFactor()
			effectiveSize := fontSize * scale
			box := GlyphBoxFor(content, baseline, effectiveSize, nextState.anchor)
			result.texts = append(result.texts, scannedText{
				content: content, box: box, fontSize: effectiveSize,
				groupID: nextState.groupID, hasGroup: nextState.hasGroup,
			})
		case "line":
			if nextState.skip {
				break
			}
			x1, _ := attrFloat(tag, "x1")
			y1, _ := attrFloat(tag, "y1")
			x2, _ := attrFloat(tag, "x2")
			y2, _ := attrFloat(tag, "y2")
			appendSegment(&result, nextState, [2]float64{x1, y1}, [2]float64{x2, y2})
		case "polyline", "polygon":
			if nextState.skip {
				break
			}
			if points, ok := tagAttr(tag, "points"); ok {
				parsePoints(points, func(from, to [2]float64) {
					appendSegment(&result, nextState, from, to)
				})
				if name == "polygon" {
					if first, last, ok := pointExtremes(points); ok {
						appendSegment(&result, nextState, last, first)
					}
				}
			}
		case "rect":
			if nextState.skip {
				break
			}
			x, _ := attrFloat(tag, "x")
			y, _ := attrFloat(tag, "y")
			width, hasWidth := attrFloat(tag, "width")
			height, hasHeight := attrFloat(tag, "height")
			if hasWidth && hasHeight && width > 0 && height > 0 {
				appendOutline(&result, nextState, [][2]float64{
					{x, y}, {x + width, y}, {x + width, y + height}, {x, y + height},
				})
			}
		case "circle":
			if nextState.skip {
				break
			}
			cx, _ := attrFloat(tag, "cx")
			cy, _ := attrFloat(tag, "cy")
			radius, ok := attrFloat(tag, "r")
			if ok && radius > 0 {
				// 圆按外接正方形的四条边近似：只用于判断文字是否被线条压住。
				appendOutline(&result, nextState, [][2]float64{
					{cx - radius, cy - radius}, {cx + radius, cy - radius},
					{cx + radius, cy + radius}, {cx - radius, cy + radius},
				})
			}
		case "ellipse":
			if nextState.skip {
				break
			}
			cx, _ := attrFloat(tag, "cx")
			cy, _ := attrFloat(tag, "cy")
			rx, okX := attrFloat(tag, "rx")
			ry, okY := attrFloat(tag, "ry")
			if okX && okY && rx > 0 && ry > 0 {
				appendOutline(&result, nextState, [][2]float64{
					{cx - rx, cy - ry}, {cx + rx, cy - ry},
					{cx + rx, cy + ry}, {cx - rx, cy + ry},
				})
			}
		case "path":
			if nextState.skip {
				break
			}
			if d, ok := tagAttr(tag, "d"); ok {
				parsePath(d, func(from, to [2]float64) {
					appendSegment(&result, nextState, from, to)
				}, func(reason string) { note(reason) })
			}
		}
	}

	return result
}

// appendOutline 追加闭合轮廓的各条边。
func appendOutline(result *scanResult, state scanState, points [][2]float64) {
	for i := range points {
		appendSegment(result, state, points[i], points[(i+1)%len(points)])
	}
}

func appendSegment(result *scanResult, state scanState, from, to [2]float64) {
	from = state.matrix.apply(from)
	to = state.matrix.apply(to)
	if math.Hypot(to[0]-from[0], to[1]-from[1]) < 1e-9 {
		return
	}
	result.segments = append(result.segments, scannedSegment{
		from: from, to: to,
		groupID: state.groupID, hasGroup: state.hasGroup, leader: state.leader,
	})
}

func parsePoints(points string, emit func(from, to [2]float64)) {
	fields := strings.FieldsFunc(points, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
	var parsed [][2]float64
	for i := 0; i+1 < len(fields); i += 2 {
		x, errX := strconv.ParseFloat(fields[i], 64)
		y, errY := strconv.ParseFloat(fields[i+1], 64)
		if errX != nil || errY != nil {
			return
		}
		parsed = append(parsed, [2]float64{x, y})
	}
	for i := 0; i+1 < len(parsed); i++ {
		emit(parsed[i], parsed[i+1])
	}
}

func pointExtremes(points string) (first, last [2]float64, ok bool) {
	fields := strings.FieldsFunc(points, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
	var parsed [][2]float64
	for i := 0; i+1 < len(fields); i += 2 {
		x, errX := strconv.ParseFloat(fields[i], 64)
		y, errY := strconv.ParseFloat(fields[i+1], 64)
		if errX != nil || errY != nil {
			return [2]float64{}, [2]float64{}, false
		}
		parsed = append(parsed, [2]float64{x, y})
	}
	if len(parsed) < 3 {
		return [2]float64{}, [2]float64{}, false
	}
	return parsed[0], parsed[len(parsed)-1], true
}

// parsePath 解析 path 的 d 属性；曲线按 8 段折线采样，弧线降级为弦并记未量测。
//
//nolint:gocognit // SVG 路径命令的状态机，按命令分派本就分支密集
func parsePath(d string, emit func(from, to [2]float64), note func(reason string)) {
	type command struct {
		letter byte
		args   []float64
	}
	var commands []command
	current := command{}
	for _, field := range tokenizePath(d) {
		if len(field) == 1 && strings.ContainsAny(field, "MmLlHhVvCcSsQqTtAaZz") {
			if current.letter != 0 {
				commands = append(commands, current)
			}
			current = command{letter: field[0]}
			continue
		}
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return
		}
		current.args = append(current.args, value)
	}
	if current.letter != 0 {
		commands = append(commands, current)
	}

	position := [2]float64{}
	start := [2]float64{}
	var lastControl [2]float64
	hasControl := false

	for _, cmd := range commands {
		relative := cmd.letter >= 'a' && cmd.letter <= 'z'
		letter := cmd.letter
		if relative {
			letter = letter - 'a' + 'A'
		}
		args := cmd.args
		step := 2
		switch letter {
		case 'H', 'V':
			step = 1
		case 'C':
			step = 6
		case 'S', 'Q':
			step = 4
		case 'T':
			step = 2
		case 'A':
			step = 7
		}
		if letter == 'Z' {
			if position != start {
				emit(position, start)
				position = start
			}
			hasControl = false
			continue
		}
		if len(args) < step || step == 0 || len(args)%step != 0 {
			return
		}
		for i := 0; i+step <= len(args); i += step {
			point := func(offset int) [2]float64 {
				p := [2]float64{args[i+offset], args[i+offset+1]}
				if relative {
					return [2]float64{position[0] + p[0], position[1] + p[1]}
				}
				return p
			}
			switch letter {
			case 'M':
				target := point(0)
				if i == 0 && letter == 'M' {
					position = target
					start = target
				} else {
					emit(position, target)
					position = target
				}
				hasControl = false
			case 'L':
				target := point(0)
				emit(position, target)
				position = target
				hasControl = false
			case 'H':
				x := args[i]
				if relative {
					x += position[0]
				}
				target := [2]float64{x, position[1]}
				emit(position, target)
				position = target
				hasControl = false
			case 'V':
				y := args[i]
				if relative {
					y += position[1]
				}
				target := [2]float64{position[0], y}
				emit(position, target)
				position = target
				hasControl = false
			case 'C':
				c1, c2, end := point(0), point(2), point(4)
				sampleCubic(position, c1, c2, end, emit)
				lastControl, hasControl = c2, true
				position = end
			case 'S':
				c2, end := point(0), point(2)
				c1 := position
				if hasControl {
					c1 = [2]float64{2*position[0] - lastControl[0], 2*position[1] - lastControl[1]}
				}
				sampleCubic(position, c1, c2, end, emit)
				lastControl, hasControl = c2, true
				position = end
			case 'Q':
				c1, end := point(0), point(2)
				sampleQuadratic(position, c1, end, emit)
				lastControl, hasControl = c1, true
				position = end
			case 'T':
				end := point(0)
				c1 := position
				if hasControl {
					c1 = [2]float64{2*position[0] - lastControl[0], 2*position[1] - lastControl[1]}
				}
				sampleQuadratic(position, c1, end, emit)
				lastControl, hasControl = c1, true
				position = end
			case 'A':
				end := [2]float64{args[i+5], args[i+6]}
				if relative {
					end = [2]float64{position[0] + end[0], position[1] + end[1]}
				}
				note("弧线按端点弦近似量测")
				emit(position, end)
				position = end
				hasControl = false
			}
		}
	}
}

func sampleCubic(from, c1, c2, to [2]float64, emit func(from, to [2]float64)) {
	previous := from
	for i := 1; i <= 8; i++ {
		t := float64(i) / 8
		u := 1 - t
		point := [2]float64{
			u*u*u*from[0] + 3*u*u*t*c1[0] + 3*u*t*t*c2[0] + t*t*t*to[0],
			u*u*u*from[1] + 3*u*u*t*c1[1] + 3*u*t*t*c2[1] + t*t*t*to[1],
		}
		emit(previous, point)
		previous = point
	}
}

func sampleQuadratic(from, c, to [2]float64, emit func(from, to [2]float64)) {
	previous := from
	for i := 1; i <= 8; i++ {
		t := float64(i) / 8
		u := 1 - t
		point := [2]float64{
			u*u*from[0] + 2*u*t*c[0] + t*t*to[0],
			u*u*from[1] + 2*u*t*c[1] + t*t*to[1],
		}
		emit(previous, point)
		previous = point
	}
}

func tokenizePath(d string) []string {
	return strings.FieldsFunc(d, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
}

// ---------- 判定 ----------

// CheckFigure 量测一张 SVG 并报告渲染层面才可见的问题。
func CheckFigure(svg string, opts CheckOptions) (CheckReport, error) {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultSVGMaxBytes
	}
	if err := AssertSafeSVG(svg, maxBytes); err != nil {
		return CheckReport{}, err
	}
	viewport := ResolveViewport(RootTag(svg))
	scaleX, scaleY := viewport.ScaleX, viewport.ScaleY
	if scaleX <= 0 {
		scaleX = MMPerUserUnit
	}
	if scaleY <= 0 {
		scaleY = MMPerUserUnit
	}
	toMM := func(p [2]float64) [2]float64 {
		return [2]float64{(p[0] - viewport.OriginX) * scaleX, (p[1] - viewport.OriginY) * scaleY}
	}

	scan := scanSVG(svg)
	report := CheckReport{TextCount: len(scan.texts)}

	if width := viewport.ViewportWidthMm; width != nil {
		report.WidthMm = *width
	}
	if height := viewport.ViewportHeightMm; height != nil {
		report.HeightMm = *height
	}

	texts := make([]scannedText, len(scan.texts))
	minFont := math.Inf(1)
	for i, text := range scan.texts {
		texts[i] = text
		texts[i].box = mmBox(text.box.MinX, text.box.MinY, text.box.MaxX, text.box.MaxY, toMM)
		texts[i].fontSize = math.Abs(text.fontSize) * math.Sqrt(scaleX*scaleY)
		height := texts[i].fontSize * (GlyphAscentRatio + GlyphDescentRatio)
		if height < minFont {
			minFont = height
		}
	}
	if !math.IsInf(minFont, 1) {
		report.MinFontMm = minFont
	}
	segments := make([]scannedSegment, len(scan.segments))
	for i, segment := range scan.segments {
		segments[i] = scannedSegment{
			from: toMM(segment.from), to: toMM(segment.to),
			groupID: segment.groupID, hasGroup: segment.hasGroup, leader: segment.leader,
		}
	}

	clearance := opts.TextClearanceMm
	if clearance == 0 && opts.FigureType != "" {
		clearance = ClearanceMm(opts.FigureType)
	}

	crossed, crossedFindings := crossedTextFindings(texts, segments)
	report.Findings = append(report.Findings, crossedFindings...)
	report.Findings = append(report.Findings, clearanceFindings(texts, segments, crossed, clearance)...)
	report.Findings = append(report.Findings, fontFindings(texts, opts.MinFontMm)...)
	report.Findings = append(report.Findings, canvasFindings(segments, report.WidthMm, report.HeightMm)...)

	for _, reason := range scan.notMeasured {
		report.Findings = append(report.Findings, CheckFinding{
			Kind: CheckNotMeasured, Blocking: false,
			Message: "未量测：" + reason,
		})
	}
	return report, nil
}

func mmBox(minX, minY, maxX, maxY float64, toMM func([2]float64) [2]float64) GlyphBox {
	cornerA := toMM([2]float64{minX, minY})
	cornerB := toMM([2]float64{maxX, maxY})
	return GlyphBox{
		MinX: math.Min(cornerA[0], cornerB[0]), MaxX: math.Max(cornerA[0], cornerB[0]),
		MinY: math.Min(cornerA[1], cornerB[1]), MaxY: math.Max(cornerA[1], cornerB[1]),
	}
}

func sameGroup(text scannedText, segment scannedSegment) bool {
	return text.hasGroup && segment.hasGroup && text.groupID == segment.groupID
}

func stripTags(text string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range text {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

// crossedTextFindings 判定文字是否被线条穿过，并返回被穿过的文字下标。
func crossedTextFindings(texts []scannedText, segments []scannedSegment) ([]bool, []CheckFinding) {
	crossed := make([]bool, len(texts))
	var findings []CheckFinding
	for i := range texts {
		for _, segment := range segments {
			if segment.leader || sameGroup(texts[i], segment) {
				continue
			}
			if !BoxCrossedBySegment(texts[i].box, segment.from, segment.to) {
				continue
			}
			crossed[i] = true
			findings = append(findings, CheckFinding{
				Kind: CheckTextCrossed, Blocking: true,
				Message: fmt.Sprintf("图面文字「%s」被线条穿过：引线或轮廓线压进字身，标号读不出；把线移到文字占位框之外", texts[i].content),
			})
			break
		}
	}
	return crossed, findings
}

// clearanceFindings 判定文字与图线的净距；已被判穿字的文字不重复报。
func clearanceFindings(texts []scannedText, segments []scannedSegment, crossed []bool, clearance float64) []CheckFinding {
	if clearance <= 0 {
		return nil
	}
	var findings []CheckFinding
	for i := range texts {
		if crossed[i] {
			continue
		}
		box := BoxQuad(texts[i].box)
		inflated := InflateQuad(box, clearance)
		nearest := math.Inf(1)
		for _, segment := range segments {
			if segment.leader || sameGroup(texts[i], segment) {
				continue
			}
			if !QuadCrossedBySegment(inflated, segment.from, segment.to) {
				continue
			}
			nearest = math.Min(nearest, QuadSegmentDistance(box, segment.from, segment.to))
		}
		if math.IsInf(nearest, 1) {
			continue
		}
		findings = append(findings, CheckFinding{
			Kind: CheckTextClearance, Blocking: true,
			Message: fmt.Sprintf("图面文字「%s」距最近图线仅 %.1f 毫米，不足 %.1f 毫米净距：标号与元件名周围要留出可读的空白",
				texts[i].content, nearest, clearance),
		})
	}
	return findings
}

func fontFindings(texts []scannedText, minFontMm float64) []CheckFinding {
	if minFontMm <= 0 {
		return nil
	}
	var findings []CheckFinding
	for _, text := range texts {
		height := text.fontSize * (GlyphAscentRatio + GlyphDescentRatio)
		if height < minFontMm {
			findings = append(findings, CheckFinding{
				Kind: CheckFontBelowMin, Blocking: true,
				Message: fmt.Sprintf("图面文字「%s」字高 %.1f 毫米，低于内控下限 %.1f 毫米", text.content, height, minFontMm),
			})
		}
	}
	return findings
}

func canvasFindings(segments []scannedSegment, widthMm, heightMm float64) []CheckFinding {
	if widthMm <= 0 || heightMm <= 0 {
		return nil
	}
	const tolerance = 0.5
	outside := func(p [2]float64) bool {
		return p[0] < -tolerance || p[1] < -tolerance ||
			p[0] > widthMm+tolerance || p[1] > heightMm+tolerance
	}
	for _, segment := range segments {
		if outside(segment.from) || outside(segment.to) {
			return []CheckFinding{{
				Kind: CheckInkOutside, Blocking: true,
				Message: "图面元素越出画布：越界部分不会被渲染出来，检查画布尺寸或元素坐标",
			}}
		}
	}
	return nil
}
