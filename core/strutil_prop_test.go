package core

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

var propAtoms = []string{"a", "b", "Z", " ", "\t", "\n", "\r", "\r\n", "世", "é", "́", "️", "‍", "​", "😀", "é", "0"}

func propString(r *rand.Rand) string {
	var sb strings.Builder
	for range r.Intn(10) {
		sb.WriteString(propAtoms[r.Intn(len(propAtoms))])
	}
	return sb.String()
}

func TestWidthPromisesProperty(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for range 20000 {
		s, n := propString(r), r.Intn(8)+1
		if g := TruncateLine(s, n); !utf8.ValidString(g) || DisplayWidth(g) > n {
			t.Fatalf("TruncateLine(%q,%d)=%q width %d", s, n, g, DisplayWidth(g))
		}
		if g := Truncate(s, n); DisplayWidth(g) > n {
			t.Fatalf("Truncate(%q,%d)=%q", s, n, g)
		}
		if g := PadLeft(s, n); DisplayWidth(g) != n {
			t.Fatalf("PadLeft(%q,%d)=%q width %d", s, n, g, DisplayWidth(g))
		}
		if g := PadRight(s, n); DisplayWidth(g) != n {
			t.Fatalf("PadRight(%q,%d)=%q width %d", s, n, g, DisplayWidth(g))
		}
		for _, l := range WrapText(s, n) {
			if DisplayWidth(l) > max(n, 2) {
				t.Fatalf("WrapText(%q,%d) line %q width %d", s, n, l, DisplayWidth(l))
			}
		}
	}
}

func TestLeadingZeroWidthRepros(t *testing.T) {
	if g := PadLeft("️b", 7); DisplayWidth(g) != 7 {
		t.Errorf("PadLeft: %q is %d", g, DisplayWidth(g))
	}
	if g := TruncateLine("\r\n️​a", 2); DisplayWidth(g) > 2 {
		t.Errorf("TruncateLine: %q is %d", g, DisplayWidth(g))
	}
	for _, l := range WrapText(" ́\r  ️\nb", 3) {
		if DisplayWidth(l) > 3 {
			t.Errorf("WrapText line %q is %d", l, DisplayWidth(l))
		}
	}
}
