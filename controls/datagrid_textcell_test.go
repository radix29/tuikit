package controls

import (
	"slices"
	"testing"
)

// core.FormatBytes picks its unit per cell, so a Size column mixes units: the
// sort has to compare bytes, not mantissas.
func TestCompareTextCellsScalesByteUnits(t *testing.T) {
	cells := []string{"3.0 GB", "1.2 MB", "4.0 KB", "512 B", "1023 B"}
	slices.SortFunc(cells, func(a, b string) int {
		return CompareTextCells(NewTextCellKey(a), NewTextCellKey(b), false)
	})
	if want := []string{"512 B", "1023 B", "4.0 KB", "1.2 MB", "3.0 GB"}; !slices.Equal(cells, want) {
		t.Errorf("sorted = %v, want %v", cells, want)
	}
}

// Durations sort by length, not by their text: past 99 hours the hour field
// widens, and an interval column mixes day counts with clocks.
func TestCompareTextCellsOrdersDurations(t *testing.T) {
	cells := []string{"100:00:00", "1 day", "99:00:00", "00:00:05.5", "2 days 00:00:01", "00:00:05", "-00:00:01"}
	slices.SortFunc(cells, func(a, b string) int {
		return CompareTextCells(NewTextCellKey(a), NewTextCellKey(b), false)
	})
	want := []string{"-00:00:01", "00:00:05", "00:00:05.5", "1 day", "2 days 00:00:01", "99:00:00", "100:00:00"}
	if !slices.Equal(cells, want) {
		t.Errorf("sorted = %v, want %v", cells, want)
	}
}

// Numbers by value ahead of text, text case-insensitively, and the
// placeholders last in both directions.
func TestCompareTextCellsOrder(t *testing.T) {
	cells := []string{"…", "beta", "10", "Alpha", "N/A", "9", "", "1,000"}
	sorted := func(desc bool) []string {
		c := slices.Clone(cells)
		slices.SortStableFunc(c, func(a, b string) int {
			return CompareTextCells(NewTextCellKey(a), NewTextCellKey(b), desc)
		})
		return c
	}
	if got, want := sorted(false), []string{"9", "10", "1,000", "Alpha", "beta", "…", "N/A", ""}; !slices.Equal(got, want) {
		t.Errorf("ascending = %q, want %q", got, want)
	}
	if got, want := sorted(true), []string{"beta", "Alpha", "1,000", "10", "9", "…", "N/A", ""}; !slices.Equal(got, want) {
		t.Errorf("descending = %q, want %q", got, want)
	}
	// The zero key is a blank: a host's row too short for the column.
	if CompareTextCells(TextCellKey{}, NewTextCellKey("1"), true) != 1 {
		t.Error("the zero TextCellKey should sort after a number, descending too")
	}
}

// ClassifyTextCell reads rendered figures as numbers (separators, units,
// percentages), placeholders as blank and names, versions and dates as text.
func TestClassifyTextCell(t *testing.T) {
	for cell, want := range map[string]CellClass{
		"1,200": CellNumber, "9 MB": CellNumber, "4.0 KB": CellNumber,
		"45%": CellNumber, "-7": CellNumber, " 12 ": CellNumber,
		"…": CellBlank, "N/A": CellBlank, "": CellBlank, "-": CellBlank, "—": CellBlank,
		"dbo.a": CellText, "16.0.4135.4": CellText, "2026-11-19": CellText,
		"2026-11-19 14:00:01": CellText, "10:30": CellText, "True": CellText,
		// Clock-style durations are numbers (core.ParseClock).
		"14:00:01": CellNumber, "100:00:00": CellNumber, "2 days 03:04:05": CellNumber,
		"00:00:01.5": CellNumber, "1 day": CellNumber,
		// Past time.Duration's range it is text, never a wrapped negative number.
		"99999 days 999999:00:00": CellText,
	} {
		if got := ClassifyTextCell(cell); got != want {
			t.Errorf("ClassifyTextCell(%q) = %v, want %v", cell, got, want)
		}
	}
}

// A signed Inf or a malformed digit grouping is a name or a list, not a number.
func TestNewTextCellKeyNotNumber(t *testing.T) {
	for _, in := range []string{"-inf", "+Inf", "1,2,3", "1,5"} {
		if k := NewTextCellKey(in); k.isNum {
			t.Errorf("NewTextCellKey(%q) read as a number", in)
		}
	}
}
