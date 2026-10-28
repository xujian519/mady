package generate

import (
	"fmt"
	"math"
	"strings"
)

// 时序图：顶部一列参与者盒，盒底引出竖直生命线，消息画成两端落在生命线上的水平
// 箭线（同步实心 / 返回虚线 / 异步空心），需要时在目标生命线上叠画激活条。
// 依据《专利审查指南》第一部分第一章 4.3：附图用黑墨线条、不得着色，参与者名写在
// 盒内、消息名写在箭线上方。生命线用细实线而非虚线——虚线留给不可见轮廓。

// SequenceParticipant 是生命线上的参与者。
type SequenceParticipant struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// SequenceMessage 是一条消息。
type SequenceMessage struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
	// Kind 为 sync（默认）/ return / async。
	Kind string `json:"kind,omitempty"`
	// Activate 为 true 时在目标生命线上画激活条。
	Activate bool `json:"activate,omitempty"`
}

// SequenceOptions 是时序图尺寸选项。
type SequenceOptions struct {
	BoxWidthMm       float64 `json:"box_width_mm,omitempty"`
	MessageSpacingMm float64 `json:"message_spacing_mm,omitempty"`
	PaddingMm        float64 `json:"padding_mm,omitempty"`
}

const (
	seqDefaultBoxWidth   = 30.0
	seqBoxHeight         = 8.0
	seqParticipantGap    = 12.0
	seqDefaultSpacing    = 10.0
	seqDefaultPadding    = 5.0
	seqMainStroke        = 0.35
	seqThinStroke        = 0.25
	seqActivationWidth   = 2.0
	seqArrowHeadLength   = 3.0
	seqArrowHeadHalfWide = 1.2
	seqReturnDash        = "1.5 1"
	seqMaxParticipants   = 50
	seqMaxMessages       = 500
)

// BuildSequenceDiagram 构造时序图 SVG。
func BuildSequenceDiagram(participants []SequenceParticipant, messages []SequenceMessage, opts SequenceOptions) (VectorFigure, error) {
	if len(participants) == 0 {
		return VectorFigure{}, newError(ErrEmptyInput, "时序图的参与者列表为空")
	}
	if len(participants) > seqMaxParticipants {
		return VectorFigure{}, newError(ErrInvalidLabel, "参与者数量超上限（%d）：%d", seqMaxParticipants, len(participants))
	}
	if len(messages) > seqMaxMessages {
		return VectorFigure{}, newError(ErrInvalidLabel, "消息数量超上限（%d）：%d", seqMaxMessages, len(messages))
	}
	boxWidth, err := positiveOption(opts.BoxWidthMm, seqDefaultBoxWidth, "box_width_mm")
	if err != nil {
		return VectorFigure{}, err
	}
	spacing, err := positiveOption(opts.MessageSpacingMm, seqDefaultSpacing, "message_spacing_mm")
	if err != nil {
		return VectorFigure{}, err
	}
	padding, err := positiveOption(opts.PaddingMm, seqDefaultPadding, "padding_mm")
	if err != nil {
		return VectorFigure{}, err
	}

	ids := make(map[string]int, len(participants))
	var centers []float64
	var labels []string
	for position, participant := range participants {
		id := strings.TrimSpace(participant.ID)
		name := strings.TrimSpace(participant.Label)
		if id == "" {
			return VectorFigure{}, newError(ErrInvalidLabel, "参与者 id 不能为空（第 %d 项）", position+1)
		}
		if name == "" {
			return VectorFigure{}, newError(ErrInvalidLabel, "参与者 %q 的名字不能为空", id)
		}
		if _, ok := ids[id]; ok {
			return VectorFigure{}, newError(ErrInvalidLabel, "参与者 id 重复：%s", id)
		}
		ids[id] = position
		centers = append(centers, padding+float64(position)*(boxWidth+seqParticipantGap)+boxWidth/2)
		labels = append(labels, name)
	}

	for position, message := range messages {
		order := position + 1
		from, to := strings.TrimSpace(message.From), strings.TrimSpace(message.To)
		if _, ok := ids[from]; !ok {
			return VectorFigure{}, newError(ErrInvalidLabel, "第 %d 条消息的起点不是已声明的参与者：%s", order, from)
		}
		if _, ok := ids[to]; !ok {
			return VectorFigure{}, newError(ErrInvalidLabel, "第 %d 条消息的终点不是已声明的参与者：%s", order, to)
		}
		if from == to {
			return VectorFigure{}, newError(ErrInvalidLabel, "第 %d 条消息的起点与终点相同，不支持自环：%s", order, from)
		}
		if strings.TrimSpace(message.Label) == "" {
			return VectorFigure{}, newError(ErrInvalidLabel, "第 %d 条消息的名字不能为空", order)
		}
		switch message.Kind {
		case "", "sync", "return", "async":
		default:
			return VectorFigure{}, newError(ErrInvalidLabel, "第 %d 条消息的类型非法：%s", order, message.Kind)
		}
		labels = append(labels, strings.TrimSpace(message.Label))
	}

	boxTop := padding
	boxBottom := boxTop + seqBoxHeight
	lifelineBottom := boxBottom + float64(max(len(messages), 1))*spacing + spacing/2

	var body []string
	for _, center := range centers {
		body = append(body, vectorLine([2]float64{center, boxBottom}, [2]float64{center, lifelineBottom}, seqThinStroke, "", ""))
	}
	for i, center := range centers {
		body = append(body,
			fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="%s" fill="none" stroke="#000000" stroke-width="%s"/>`,
				fmtMm(center-boxWidth/2), fmtMm(boxTop), fmtMm(boxWidth), fmtMm(seqBoxHeight), fmtMm(seqMainStroke)),
			vectorText(center, boxTop+seqBoxHeight/2+VectorTextMidRatio*VectorFontSizeMm,
				AnchorMiddle, VectorFontSizeMm, labels[i], ""),
		)
	}

	messageIndex := 0
	for _, message := range messages {
		y := boxBottom + float64(messageIndex+1)*spacing
		messageIndex++
		fromX := centers[ids[strings.TrimSpace(message.From)]]
		toX := centers[ids[strings.TrimSpace(message.To)]]
		if message.Activate {
			body = append(body, fmt.Sprintf(
				`<rect x="%s" y="%s" width="%s" height="%s" fill="none" stroke="#000000" stroke-width="%s"/>`,
				fmtMm(toX-seqActivationWidth/2), fmtMm(y-spacing/2), fmtMm(seqActivationWidth),
				fmtMm(spacing), fmtMm(seqThinStroke)))
		}
		body = append(body, sequenceMessageFragments(fromX, toX, y, message.Kind, strings.TrimSpace(message.Label))...)
	}

	widthMm := 2*padding + float64(len(centers))*boxWidth + float64(len(centers)-1)*seqParticipantGap
	heightMm := lifelineBottom + padding
	return VectorFigure{
		WidthMm:  widthMm,
		HeightMm: heightMm,
		SVG:      wrapVectorSVG(widthMm, heightMm, strings.Join(body, "\n")),
		Labels:   labels,
	}, nil
}

// sequenceMessageFragments 生成一条消息：箭线（箭尾止于箭头底边）、箭头与消息名。
func sequenceMessageFragments(x1, x2, y float64, kind, label string) []string {
	filled, dashed := true, false
	switch kind {
	case "return":
		dashed = true
	case "async":
		filled = false
	}
	direction := 1.0
	if x2 < x1 {
		direction = -1
	}
	baseX := x2 - direction*seqArrowHeadLength
	dash := ""
	if dashed {
		dash = seqReturnDash
	}
	fill := "none"
	if filled {
		fill = "#000000"
	}
	return []string{
		vectorLine([2]float64{x1, y}, [2]float64{baseX, y}, seqMainStroke, dash, ""),
		fmt.Sprintf(`<polygon points="%s,%s %s,%s %s,%s" fill="%s" stroke="#000000" stroke-width="%s"/>`,
			fmtMm(x2), fmtMm(y),
			fmtMm(baseX), fmtMm(y-seqArrowHeadHalfWide),
			fmtMm(baseX), fmtMm(y+seqArrowHeadHalfWide),
			fill, fmtMm(seqThinStroke)),
		vectorText((x1+x2)/2, y-1.2, AnchorMiddle, VectorFontSizeMm, label, ""),
	}
}

// positiveOption 解析正数选项；零值取默认，负数与非有限值报错。
func positiveOption(value, fallback float64, name string) (float64, error) {
	if value == 0 {
		return fallback, nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, newError(ErrInvalidLabel, "%s 必须为正有限数：%v", name, value)
	}
	return value, nil
}
