package generate

import (
	"math"
	"strconv"
	"strings"
)

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
		// 属性值不可解析→按缺失处理（ok=false），几何核验走宽松口径。
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
				// 参数不可解析→保持单位矩阵并标记失败，由调用方跳过该段变换。
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
