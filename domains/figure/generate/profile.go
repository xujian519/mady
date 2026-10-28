package generate

import "fmt"

// 附图提交的法域规格档案（纯数据）。数值出处：
// - 中国：《专利审查指南》第五部分第一章 4.2/4.3（A4，页边距顶 25、左 25、
//   右 15、底 15 毫米）、第一部分第一章 4.3（图号「图1」标注在附图正下方，
//   两幅以上才编号）。
// - PCT：Rule 11.6(c)（上 2.5、左 2.5、右 1.5、下 1.0 厘米）、11.13(h)
//   （数字与字母高度不小于 3.2 毫米）、11.13(k)（图号冠 "Fig."，仅一幅时不编号）。
// - USPTO：37 CFR 1.84(g)（上/左 1 英寸、右 5/8、下 3/8 英寸）、1.84(p)(3)
//   （字高不小于 3.2 毫米）、1.84(u)（图号冠 "FIG."，仅一个视图时不编号）。

// Office 是目标受理局/指定局。
type Office string

// Office constants.
const (
	OfficeCNIPA Office = "cnipa"
	OfficePCT   Office = "pct"
	OfficeUSPTO Office = "uspto"
)

// AllOffices 返回支持的法域。
func AllOffices() []Office { return []Office{OfficeCNIPA, OfficePCT, OfficeUSPTO} }

// Valid reports whether the office is supported.
func (o Office) Valid() bool {
	switch o {
	case OfficeCNIPA, OfficePCT, OfficeUSPTO:
		return true
	default:
		return false
	}
}

// PaperSize 是幅面尺寸（毫米）。
type PaperSize struct {
	WidthMm  float64 `json:"width_mm"`
	HeightMm float64 `json:"height_mm"`
}

// PageMargins 是四边页边距（毫米）。
type PageMargins struct {
	TopMm    float64 `json:"top_mm"`
	LeftMm   float64 `json:"left_mm"`
	RightMm  float64 `json:"right_mm"`
	BottomMm float64 `json:"bottom_mm"`
}

// OfficeProfile 是一个法域的附图提交规格。
type OfficeProfile struct {
	Office  Office      `json:"office"`
	Paper   PaperSize   `json:"paper"`
	Margins PageMargins `json:"margins"`
	// MinCharHeightMm 为图中数字与字母的最小字高；0 表示该法域无具体数值。
	MinCharHeightMm float64 `json:"min_char_height_mm,omitempty"`
	// CaptionStyle 为图号写法：cnipa「图1」、pct「Fig. 1」、uspto「FIG. 1」。
	CaptionStyle string `json:"caption_style"`
	// CaptionOnlyWhenMultiple 表示仅一幅附图时不编号（三国均如此）。
	CaptionOnlyWhenMultiple bool `json:"caption_only_when_multiple"`
	// SheetNumbering 为页码写法：figure-pages（第2页 共3页）或 sheet-of（2/3）。
	SheetNumbering string `json:"sheet_numbering"`
	// ReductionRatio 为「缩小到该比例仍须清晰可辨」（三分之二）。
	ReductionRatio float64 `json:"reduction_ratio"`
}

// a4 是 A4 幅面（毫米）。
var a4 = PaperSize{WidthMm: 210, HeightMm: 297}

var officeProfiles = map[Office]OfficeProfile{
	OfficeCNIPA: {
		Office: OfficeCNIPA, Paper: a4,
		Margins:                 PageMargins{TopMm: 25, LeftMm: 25, RightMm: 15, BottomMm: 15},
		CaptionStyle:            "figure-number",
		CaptionOnlyWhenMultiple: true,
		SheetNumbering:          "figure-pages",
		ReductionRatio:          2.0 / 3.0,
	},
	OfficePCT: {
		Office: OfficePCT, Paper: a4,
		Margins:                 PageMargins{TopMm: 25, LeftMm: 25, RightMm: 15, BottomMm: 10},
		MinCharHeightMm:         3.2,
		CaptionStyle:            "fig",
		CaptionOnlyWhenMultiple: true,
		SheetNumbering:          "sheet-of",
		ReductionRatio:          2.0 / 3.0,
	},
	OfficeUSPTO: {
		Office: OfficeUSPTO, Paper: a4,
		Margins:                 PageMargins{TopMm: 25, LeftMm: 25, RightMm: 15, BottomMm: 10},
		MinCharHeightMm:         3.2,
		CaptionStyle:            "fig-upper",
		CaptionOnlyWhenMultiple: true,
		SheetNumbering:          "sheet-of",
		ReductionRatio:          2.0 / 3.0,
	},
}

// OfficeProfileFor 返回法域的规格档案。
func OfficeProfileFor(office Office) (OfficeProfile, error) {
	profile, ok := officeProfiles[office]
	if !ok {
		return OfficeProfile{}, newError(ErrInvalidPage, "未知的法域 %q（可选 cnipa/pct/uspto）", office)
	}
	return profile, nil
}

// FigureCaption 生成图号文字；本案附图不足两幅时返回空（三国都只在多幅时编号）。
func FigureCaption(office Office, figureNumber, figureCount int) (string, error) {
	profile, err := OfficeProfileFor(office)
	if err != nil {
		return "", err
	}
	if figureNumber < 1 {
		return "", newError(ErrInvalidPage, "图号必须是正整数：%d", figureNumber)
	}
	if figureCount < figureNumber {
		return "", newError(ErrInvalidPage, "附图总数必须不小于图号：%d < %d", figureCount, figureNumber)
	}
	if profile.CaptionOnlyWhenMultiple && figureCount < 2 {
		return "", nil
	}
	switch profile.CaptionStyle {
	case "fig":
		return fmt.Sprintf("Fig. %d", figureNumber), nil
	case "fig-upper":
		return fmt.Sprintf("FIG. %d", figureNumber), nil
	default:
		return fmt.Sprintf("图%d", figureNumber), nil
	}
}

// SheetNumberText 生成附图页页码；中国为「第2页 共3页」，PCT/USPTO 为「2/3」。
func SheetNumberText(office Office, sheetIndex, sheetTotal int) (string, error) {
	profile, err := OfficeProfileFor(office)
	if err != nil {
		return "", err
	}
	if sheetIndex < 1 {
		return "", newError(ErrInvalidPage, "附图页序号必须是正整数：%d", sheetIndex)
	}
	if sheetTotal < sheetIndex {
		return "", newError(ErrInvalidPage, "附图页总数必须不小于序号：%d < %d", sheetTotal, sheetIndex)
	}
	if profile.SheetNumbering == "sheet-of" {
		return fmt.Sprintf("%d/%d", sheetIndex, sheetTotal), nil
	}
	if sheetTotal > 1 {
		return fmt.Sprintf("第%d页 共%d页", sheetIndex, sheetTotal), nil
	}
	return fmt.Sprintf("第%d页", sheetIndex), nil
}
