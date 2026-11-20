package controls

import (
	"cmp"
	"strings"

	"github.com/radix29/tuikit/core"
)

// datagrid_textcell.go reads a pre-rendered text cell for a host's column sort
// and for SetAutoAlign: a grid filled with formatted strings ("1,234",
// "4.0 KB", "45%", "…") sorts and aligns by what the strings say, not by their
// bytes. A grid whose host still holds typed values (a query result) compares
// those instead and needs none of this.

// TextCellKey is one cell read for sorting: a number when the whole cell is
// one (core.ParseFormattedNumber: separators, a trailing unit or %) or is a
// clock-style duration (core.ParseClock: "01:02:03", "2 days 03:04:05",
// read as seconds, so it sorts and aligns as a number), its case-folded text
// otherwise, or blank for a cell that holds no value yet or none at all (see
// NewTextCellKey). The zero value is a blank, which a host can use for a row
// too short to have the column.
//
// Durations read as numbers since 2026-11-20 (pgMS's interval columns): until
// then "00:01:02" was text, left-aligned and compared as a string — the same
// order for zero-padded FormatClock cells under 100 hours, which is why the
// change reorders nothing an existing host showed except past 99:59:59.
type TextCellKey struct {
	filled bool
	isNum  bool
	num    float64
	text   string
}

// textCellPlaceholders are the cells that stand for a missing value: a
// backfill not landed yet ("…", "..."), one that failed or does not apply
// ("N/A", a dash), or an empty cell. Typography, not any one host's choice.
var textCellPlaceholders = map[string]bool{
	"": true, "…": true, "...": true, "N/A": true, "-": true, "—": true,
}

// NewTextCellKey reads cell, surrounding spaces ignored. "" "…" "..." "N/A"
// "-" and "—" are blanks.
func NewTextCellKey(cell string) TextCellKey {
	s := strings.TrimSpace(cell)
	if textCellPlaceholders[s] {
		return TextCellKey{}
	}
	// A duration first: "1 day" is a day, which ParseFormattedNumber would read
	// as the bare 1 with its unit stripped.
	if d, ok := core.ParseClock(s); ok {
		return TextCellKey{filled: true, isNum: true, num: d.Seconds()}
	}
	if n, ok := core.ParseFormattedNumber(s); ok {
		return TextCellKey{filled: true, isNum: true, num: n}
	}
	return TextCellKey{filled: true, text: strings.ToLower(s)}
}

// Class is k as a SetAutoAlign classifier reads it.
func (k TextCellKey) Class() CellClass {
	switch {
	case !k.filled:
		return CellBlank
	case k.isNum:
		return CellNumber
	}
	return CellText
}

// ClassifyTextCell is a ready SetAutoAlign cell classifier for a grid of
// pre-rendered text: numbers as NewTextCellKey reads them, so the columns
// that sort as numbers are the ones that align as numbers.
func ClassifyTextCell(cell string) CellClass {
	return NewTextCellKey(cell).Class()
}

// CompareTextCells orders a before b ascending, or the reverse for desc:
// numbers by value ahead of text, text case-insensitively. Blanks sort last
// either way, so reversing a half-loaded column doesn't bring every "…" to the
// top. Use it with a stable sort, so rows tied on the column keep their order.
func CompareTextCells(a, b TextCellKey, desc bool) int {
	if !a.filled || !b.filled {
		switch {
		case !a.filled && !b.filled:
			return 0
		case !a.filled:
			return 1
		default:
			return -1
		}
	}
	var c int
	switch {
	case a.isNum && b.isNum:
		c = cmp.Compare(a.num, b.num)
	case a.isNum:
		c = -1
	case b.isNum:
		c = 1
	default:
		c = strings.Compare(a.text, b.text)
	}
	if desc {
		return -c
	}
	return c
}
