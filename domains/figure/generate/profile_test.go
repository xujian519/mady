package generate

import "testing"

func TestOfficeProfiles(t *testing.T) {
	cases := []struct {
		office     Office
		margins    PageMargins
		minChar    float64
		sheetStyle string
	}{
		{OfficeCNIPA, PageMargins{TopMm: 25, LeftMm: 25, RightMm: 15, BottomMm: 15}, 0, "figure-pages"},
		{OfficePCT, PageMargins{TopMm: 25, LeftMm: 25, RightMm: 15, BottomMm: 10}, 3.2, "sheet-of"},
		{OfficeUSPTO, PageMargins{TopMm: 25, LeftMm: 25, RightMm: 15, BottomMm: 10}, 3.2, "sheet-of"},
	}
	for _, tc := range cases {
		profile, err := OfficeProfileFor(tc.office)
		if err != nil {
			t.Fatalf("%s: %v", tc.office, err)
		}
		if profile.Margins != tc.margins {
			t.Errorf("%s margins = %+v, want %+v", tc.office, profile.Margins, tc.margins)
		}
		if profile.MinCharHeightMm != tc.minChar {
			t.Errorf("%s minChar = %v, want %v", tc.office, profile.MinCharHeightMm, tc.minChar)
		}
		if profile.SheetNumbering != tc.sheetStyle {
			t.Errorf("%s sheet style = %q, want %q", tc.office, profile.SheetNumbering, tc.sheetStyle)
		}
		if profile.Paper != a4 {
			t.Errorf("%s paper = %+v, want A4", tc.office, profile.Paper)
		}
		if !profile.CaptionOnlyWhenMultiple {
			t.Errorf("%s should only caption multiple figures", tc.office)
		}
		if !almost(profile.ReductionRatio, 2.0/3.0) {
			t.Errorf("%s reduction = %v, want 2/3", tc.office, profile.ReductionRatio)
		}
	}
}

func TestOfficeProfileUnknown(t *testing.T) {
	if _, err := OfficeProfileFor("epo"); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("err = %v, want invalid_page", err)
	}
	if Office("epo").Valid() {
		t.Fatal("epo should be invalid")
	}
	if !OfficeCNIPA.Valid() || !OfficePCT.Valid() || !OfficeUSPTO.Valid() {
		t.Fatal("cnipa/pct/uspto should be valid")
	}
}

func TestFigureCaption(t *testing.T) {
	cases := []struct {
		office Office
		number int
		count  int
		want   string
	}{
		{OfficeCNIPA, 1, 1, ""},
		{OfficePCT, 1, 1, ""},
		{OfficeCNIPA, 2, 3, "图2"},
		{OfficePCT, 2, 3, "Fig. 2"},
		{OfficeUSPTO, 3, 3, "FIG. 3"},
	}
	for _, tc := range cases {
		got, err := FigureCaption(tc.office, tc.number, tc.count)
		if err != nil {
			t.Fatalf("%s %d/%d: %v", tc.office, tc.number, tc.count, err)
		}
		if got != tc.want {
			t.Errorf("%s %d/%d = %q, want %q", tc.office, tc.number, tc.count, got, tc.want)
		}
	}
}

func TestFigureCaptionErrors(t *testing.T) {
	if _, err := FigureCaption(OfficeCNIPA, 0, 3); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("number 0: %v", err)
	}
	if _, err := FigureCaption(OfficeCNIPA, 4, 3); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("number > count: %v", err)
	}
	if _, err := FigureCaption("epo", 1, 2); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("unknown office: %v", err)
	}
}

func TestSheetNumberText(t *testing.T) {
	cases := []struct {
		office Office
		index  int
		total  int
		want   string
	}{
		{OfficeCNIPA, 2, 3, "第2页 共3页"},
		{OfficeCNIPA, 1, 1, "第1页"},
		{OfficePCT, 1, 2, "1/2"},
		{OfficeUSPTO, 3, 3, "3/3"},
	}
	for _, tc := range cases {
		got, err := SheetNumberText(tc.office, tc.index, tc.total)
		if err != nil {
			t.Fatalf("%s %d/%d: %v", tc.office, tc.index, tc.total, err)
		}
		if got != tc.want {
			t.Errorf("%s %d/%d = %q, want %q", tc.office, tc.index, tc.total, got, tc.want)
		}
	}
	if _, err := SheetNumberText(OfficeCNIPA, 0, 2); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("index 0: %v", err)
	}
	if _, err := SheetNumberText(OfficeCNIPA, 3, 2); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("index > total: %v", err)
	}
	if _, err := SheetNumberText("epo", 1, 2); errorCode(t, err) != ErrInvalidPage {
		t.Fatalf("unknown office: %v", err)
	}
}
