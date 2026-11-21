package core

import (
	"slices"
	"strings"
	"testing"
)

// A wrapped entry continues under its second column, not under the key or at
// the margin.
func TestWrapHangingHangsUnderSecondColumn(t *testing.T) {
	got, src := WrapHanging([]string{"  Ctrl+X      one two three four five"}, 30)
	want := []string{
		"  Ctrl+X      one two three",
		"              four five",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if !slices.Equal(src, []int{0, 0}) {
		t.Errorf("src = %v, want [0 0]", src)
	}
}

// src maps every row, continuations included, to its source line.
func TestWrapHangingMapsRowsToSource(t *testing.T) {
	_, src := WrapHanging([]string{
		"Heading  with a gap that wraps here",
		"",
		"  Ctrl+X      one two three four five",
	}, 30)
	if want := []int{0, 0, 1, 2, 2}; !slices.Equal(src, want) {
		t.Errorf("src = %v, want %v", src, want)
	}
}

// Every row fits w and the rows keep every word in order; a line with no
// space to break at is cut mid-word rather than overrunning.
func TestWrapHangingFitsAndLosesNothing(t *testing.T) {
	lines := []string{
		"  F5          Execute the query, or the selected text if there is one",
		"  Unbreakable_token_much_wider_than_the_area",
		"Heading",
		"    deeply indented entry with no two-space gap at all",
	}
	want := strings.Fields(strings.Join(lines, " "))
	for w := 12; w <= 80; w++ {
		rows, _ := WrapHanging(lines, w)
		for _, r := range rows {
			if DisplayWidth(r) > w {
				t.Fatalf("w=%d: row %q is wider", w, r)
			}
		}
		got := strings.Fields(strings.Join(rows, " "))
		if strings.Join(got, "") != strings.Join(want, "") {
			t.Fatalf("w=%d: lost or reordered text: %q", w, rows)
		}
	}
}

// A hang deeper than half the width falls back to the indent, so the
// continuation keeps room for words.
func TestWrapHangingDeepHangFallsBack(t *testing.T) {
	got, _ := WrapHanging([]string{"  A_very_long_key_name  one two three"}, 30)
	if len(got) < 2 || leadingSpaces(got[1]) != 2 {
		t.Errorf("got %q, want the continuation at the indent (2)", got)
	}
}

func TestWrapHangingNoWidth(t *testing.T) {
	in := []string{"  a  b c d e f g h", "x"}
	got, src := WrapHanging(in, 0)
	if !slices.Equal(got, in) || !slices.Equal(src, []int{0, 1}) {
		t.Errorf("w=0: got %q %v, want the lines unchanged", got, src)
	}
}

// Rows never cut inside a grapheme cluster: a ZWJ family measures 2 columns and
// a skin-tone modifier stays with its base.
func TestWrapHangingKeepsGraphemeClusters(t *testing.T) {
	for _, tc := range []struct {
		line string
		w    int
	}{
		{"ab 👨\u200d👩\u200d👧 cdefgh", 5},
		{"abcd👍🏽xyz", 6},
	} {
		rows, _ := WrapHanging([]string{tc.line}, tc.w)
		for _, r := range rows {
			if DisplayWidth(r) > tc.w {
				t.Errorf("%q w=%d: row %q is wider", tc.line, tc.w, r)
			}
			if strings.HasPrefix(r, "🏽") || strings.HasPrefix(r, "👧") || strings.HasPrefix(r, "\u200d") {
				t.Errorf("%q w=%d: row %q starts mid-cluster", tc.line, tc.w, r)
			}
		}
		if got := strings.ReplaceAll(strings.Join(rows, ""), " ", ""); got != strings.ReplaceAll(tc.line, " ", "") {
			t.Errorf("%q w=%d: rejoined %q lost text (rows %q)", tc.line, tc.w, got, rows)
		}
	}
	// The family is one 2-column cluster, so "ab 👨\u200d👩\u200d👧" is 5 columns, not 9.
	if DisplayWidth("ab 👨\u200d👩\u200d👧") != 5 {
		t.Fatal("family should measure 2 columns")
	}
	rows, _ := WrapHanging([]string{"xy 👨\u200d👩\u200d👧"}, 5)
	if len(rows) != 1 {
		t.Errorf("a 5-column line wrapped at w=5: %q", rows)
	}
}
