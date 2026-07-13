package core

import (
	"strings"
	"testing"
)

func TestDiffCellsNoChange(t *testing.T) {
	old := ParseLine("hello world")
	new := ParseLine("hello world")
	diff := DiffCells(old, new)
	if len(diff.Segments) != 0 || diff.ClearTail {
		t.Fatalf("expected no diff, got %+v", diff)
	}
}

func TestDiffCellsSingleCell(t *testing.T) {
	old := ParseLine("hello world")
	new := ParseLine("hello World")
	diff := DiffCells(old, new)
	if len(diff.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(diff.Segments))
	}
	seg := diff.Segments[0]
	if seg.StartCol != 6 {
		t.Fatalf("start col = %d, want 6", seg.StartCol)
	}
	if len(seg.Cells) != 1 || seg.Cells[0].Rune != 'W' {
		t.Fatalf("expected single 'W' cell, got %+v", seg.Cells)
	}
	if diff.ClearTail {
		t.Fatal("unexpected tail clear")
	}
}

func TestDiffCellsPrefixChange(t *testing.T) {
	old := ParseLine("hello world")
	new := ParseLine("Hello world")
	diff := DiffCells(old, new)
	seg := diff.Segments[0]
	if seg.StartCol != 0 {
		t.Fatalf("start col = %d, want 0", seg.StartCol)
	}
	if len(seg.Cells) != 1 || seg.Cells[0].Rune != 'H' {
		t.Fatalf("expected 'H', got %+v", seg.Cells)
	}
}

func TestDiffCellsSuffixChange(t *testing.T) {
	old := ParseLine("hello world")
	new := ParseLine("hello world!")
	diff := DiffCells(old, new)
	seg := diff.Segments[0]
	if seg.StartCol != 11 {
		t.Fatalf("start col = %d, want 11", seg.StartCol)
	}
	if len(seg.Cells) != 1 || seg.Cells[0].Rune != '!' {
		t.Fatalf("expected '!', got %+v", seg.Cells)
	}
}

func TestDiffCellsShorterNew(t *testing.T) {
	old := ParseLine("hello world")
	new := ParseLine("hello")
	diff := DiffCells(old, new)
	if len(diff.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(diff.Segments))
	}
	seg := diff.Segments[0]
	if seg.StartCol != 5 {
		t.Fatalf("start col = %d, want 5", seg.StartCol)
	}
	if len(seg.Cells) != 0 {
		t.Fatalf("expected empty segment cells, got %d", len(seg.Cells))
	}
	if !diff.ClearTail || diff.TailStart != 5 {
		t.Fatalf("expected clear tail from col 5, got clear=%v start=%d", diff.ClearTail, diff.TailStart)
	}
}

func TestDiffCellsWideCharBoundary(t *testing.T) {
	red := ParseSGR("31", DefaultStyle)
	old := Row{Cells: []Cell{
		{Rune: 'a', Style: DefaultStyle},
		{Rune: '中', Style: DefaultStyle, Width: 2},
		{Rune: 0, Style: DefaultStyle, Width: 0},
		{Rune: 'b', Style: DefaultStyle},
	}}
	new := Row{Cells: []Cell{
		{Rune: 'a', Style: DefaultStyle},
		{Rune: '中', Style: red, Width: 2},
		{Rune: 0, Style: red, Width: 0},
		{Rune: 'b', Style: DefaultStyle},
	}}
	diff := DiffCells(old, new)
	seg := diff.Segments[0]
	if seg.StartCol != 1 {
		t.Fatalf("start col = %d, want 1 (primary of 中)", seg.StartCol)
	}
	// The segment should include both the primary and continuation cells.
	if len(seg.Cells) != 2 || seg.Cells[0].Rune != '中' || seg.Cells[1].Rune != 0 {
		t.Fatalf("expected 中 primary and continuation cells, got %+v", seg.Cells)
	}
}

func TestDiffCellsRawRow(t *testing.T) {
	old := Row{Raw: "old"}
	new := Row{Raw: "new"}
	diff := DiffCells(old, new)
	if len(diff.Segments) != 1 || diff.Segments[0].StartCol != 0 {
		t.Fatalf("expected full-row segment for raw rows, got %+v", diff)
	}
}

func TestDiffFrameIdentical(t *testing.T) {
	old := []Row{ParseLine("a"), ParseLine("b")}
	new := []Row{ParseLine("a"), ParseLine("b")}
	if d := DiffFrame(old, new); len(d) != 0 {
		t.Fatalf("expected no diff, got %d rows", len(d))
	}
}

func TestDiffFrameNewRow(t *testing.T) {
	old := []Row{ParseLine("a")}
	new := []Row{ParseLine("a"), ParseLine("b"), ParseLine("c")}
	diff := DiffFrame(old, new)
	if len(diff) != 2 {
		t.Fatalf("expected 2 row diffs, got %d", len(diff))
	}
	if diff[0].Row != 1 || diff[1].Row != 2 {
		t.Fatalf("unexpected rows: %+v", diff)
	}
}

func TestSerializeRowSegmentBasic(t *testing.T) {
	row := ParseLine("hello world")
	seg := row.Cells[6:11]
	ser := SerializeRowSegment(seg, DefaultStyle)
	if !strings.Contains(ser, "world") {
		t.Fatalf("expected 'world' in output, got %q", ser)
	}
	if !strings.HasPrefix(ser, "\x1b[0m") {
		t.Fatalf("expected leading reset, got %q", ser)
	}
	if strings.HasSuffix(ser, "\x1b[0m") {
		t.Fatalf("expected no trailing reset when afterStyle is default, got %q", ser)
	}
}

func TestSerializeRowSegmentStyleTransition(t *testing.T) {
	red := ParseSGR("31", DefaultStyle)
	blue := ParseSGR("34", DefaultStyle)
	row := Row{
		Cells: []Cell{
			{Rune: 'a', Style: red},
			{Rune: 'b', Style: blue},
		},
	}
	ser := SerializeRowSegment(row.Cells, red)
	if !strings.HasPrefix(ser, "\x1b[0m") {
		t.Fatalf("expected leading reset, got %q", ser)
	}
	if !strings.Contains(ser, "a") || !strings.Contains(ser, "b") {
		t.Fatalf("expected 'a' and 'b', got %q", ser)
	}
	if !strings.HasSuffix(ser, RenderSGR(blue, red)) {
		t.Fatalf("expected transition back to red, got %q", ser)
	}
}

func TestDiffCellsPreservesUnchangedStyle(t *testing.T) {
	red := ParseSGR("31", DefaultStyle)
	old := Row{Cells: []Cell{
		{Rune: 'h', Style: DefaultStyle},
		{Rune: 'i', Style: DefaultStyle},
		{Rune: '!', Style: red},
	}}
	new := Row{Cells: []Cell{
		{Rune: 'h', Style: DefaultStyle},
		{Rune: 'i', Style: red},
		{Rune: '!', Style: red},
	}}
	diff := DiffCells(old, new)
	seg := diff.Segments[0]
	if seg.StartCol != 1 {
		t.Fatalf("start col = %d, want 1", seg.StartCol)
	}
	if len(seg.Cells) != 1 || seg.Cells[0].Rune != 'i' {
		t.Fatalf("expected 'i', got %+v", seg.Cells)
	}
	if !seg.AfterStyle.Equal(red) {
		t.Fatalf("expected afterStyle red, got %+v", seg.AfterStyle)
	}
}
