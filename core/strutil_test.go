package core

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"
)

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"你好", 4},                   // CJK: 2 columns per rune
		{"é", 1},                   // "e" + combining acute accent: one grapheme cluster
		{"\U0001F1FA\U0001F1F8", 2}, // flag emoji (regional indicator pair)
	}
	for _, c := range cases {
		if got := DisplayWidth(c.s); got != c.want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},      // fits, unchanged
		{"hello world", 5, "hell…"}, // clipped with ellipsis
		{"你好吗", 3, "你…"},            // wide runes: only one fits in the budget
		{"abc", 0, ""},              // n<=0 always empty
		{"", 5, ""},                 // empty input
		// The single pass decides "fits" and "where to cut" together, so the
		// boundary either side of n is where it would go wrong first.
		{"abcde", 5, "abcde"},  // exactly n, returned whole
		{"abcdef", 5, "abcd…"}, // one column over
		{"ab", 1, "…"},         // no room for anything but the ellipsis
		{"a", 1, "a"},          // ...unless it already fits
		{"你好", 2, "…"},         // a wide grapheme straddling the budget is dropped whole
		{"你a", 3, "你a"},        // mixed widths summing to exactly n
	}
	for _, c := range cases {
		if got := Truncate(c.s, c.n); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
	}
}

func TestTruncateLeft(t *testing.T) {
	const zwj = "👨\u200d👩\u200d👧" // one 2-column cluster of five runes
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"}, // fits, unchanged
		{"abcde", 5, "abcde"},  // exactly n, returned whole
		{"abcdef", 5, "…cdef"}, // one column over: the tail survives
		{`C:\a\b\c.mdf`, 8, `…b\c.mdf`},
		{"abc", 0, ""}, // n<=0 always empty
		{"abc", -1, ""},
		{"", 5, ""},                   // empty input
		{"ab", 1, "…"},                // no room for anything but the ellipsis
		{"a", 1, "a"},                 // ...unless it already fits
		{"x你好", 4, "…好"},              // a wide grapheme straddling the budget is dropped whole
		{"你好吗", 5, "…好吗"},             // wide tail, budget used exactly
		{"ab" + zwj + "cd", 4, "…cd"}, // the ZWJ cluster would overflow: dropped whole, never split
		{"ab" + zwj + "cd", 5, "…" + zwj + "cd"},
		{"xyze\u0301", 2, "…e\u0301"}, // the combining mark stays with its base
	}
	for _, c := range cases {
		got := TruncateLeft(c.s, c.n)
		if got != c.want {
			t.Errorf("TruncateLeft(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
		if w := DisplayWidth(got); c.n > 0 && w > c.n {
			t.Errorf("TruncateLeft(%q, %d) is %d columns wide", c.s, c.n, w)
		}
	}
}

func TestPadRight(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"ab", 5, "ab   "},  // padded with trailing spaces
		{"ab", 2, "ab"},     // exact width, unchanged
		{"abcde", 3, "abc"}, // hard-clipped, no ellipsis
		{"你好", 3, "你 "},     // wide rune that doesn't fit is dropped, remainder padded
	}
	for _, c := range cases {
		if got := PadRight(c.s, c.n); got != c.want {
			t.Errorf("PadRight(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
		if got := DisplayWidth(PadRight(c.s, c.n)); got != c.n {
			t.Errorf("DisplayWidth(PadRight(%q, %d)) = %d, want exactly %d", c.s, c.n, got, c.n)
		}
	}
}

func TestPadLeft(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"ab", 5, "   ab"},  // padded with leading spaces
		{"ab", 2, "ab"},     // exact width, unchanged
		{"abcde", 3, "abc"}, // hard-clipped, no ellipsis
		{"你好", 3, "你 "},     // wide rune that doesn't fit is dropped, remainder padded
	}
	for _, c := range cases {
		if got := PadLeft(c.s, c.n); got != c.want {
			t.Errorf("PadLeft(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
		if got := DisplayWidth(PadLeft(c.s, c.n)); got != c.n {
			t.Errorf("DisplayWidth(PadLeft(%q, %d)) = %d, want exactly %d", c.s, c.n, got, c.n)
		}
	}
}

func TestFormatThousands(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{100423, "100,423"},
		{-1234567, "-1,234,567"},
		{-5, "-5"},
		// The extremes. Taking a number's digits by negating it and dividing
		// down is wrong at math.MinInt64, where the negation is a no-op and
		// the loop produces nothing: this used to answer "--".
		{math.MaxInt64, "9,223,372,036,854,775,807"},
		{math.MinInt64, "-9,223,372,036,854,775,808"},
		{math.MinInt64 + 1, "-9,223,372,036,854,775,807"},
	}
	for _, c := range cases {
		if got := FormatThousands(c.n); got != c.want {
			t.Errorf("FormatThousands(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestParseFormattedNumber(t *testing.T) {
	for in, want := range map[string]bool{
		"1,234": true, "12 MB": true, "-3": true, "45%": true, "0.5": true,
		"dbo.t": false, "Inf": false, "NaN": false, "16.0.4135.4": false,
		"2026-01-02 10:00:00": false, "0x1p3": false, "ONLINE": false,
		"": false, "1_000": false, "MB": false,
		"+Inf": false, "-inf": false, "+Infinity": false, "1,2,3": false,
		"1,,2": false, ",5": false, "-,5": false, "1,5": false, "12,34": false,
		"1,234.5,6": false, "-1,234,567.5": true, "+3": true,
		"-.5": true, ".5": true,
	} {
		if _, ok := ParseFormattedNumber(in); ok != want {
			t.Errorf("ParseFormattedNumber(%q) ok = %v, want %v", in, ok, want)
		}
	}
	// Non-byte units are stripped, not scaled.
	if n, _ := ParseFormattedNumber("250 ms"); n != 250 {
		t.Errorf(`ParseFormattedNumber("250 ms") = %v, want 250`, n)
	}
	if n, _ := ParseFormattedNumber("4.0 KB"); n != 4096 {
		t.Errorf(`ParseFormattedNumber("4.0 KB") = %v, want 4096`, n)
	}
}

// ParseFormattedNumber is FormatBytes' and FormatThousands' inverse, so the
// pair can't drift apart: every unit FormatBytes writes reads back as bytes
// (within its one-decimal rounding), in the same order as the sizes, and
// FormatThousands reads back exactly.
func TestParseFormattedNumberRoundTrips(t *testing.T) {
	var sizes []int64
	for shift := 0; shift < 63; shift++ {
		for _, f := range []float64{1, 1.3, 1.5, 1.95, 3, 7.9} {
			if v := f * float64(int64(1)<<shift); v < math.MaxInt64 {
				sizes = append(sizes, int64(v))
			}
		}
	}
	sizes = append(sizes, 0, 512, 1023, math.MaxInt64)
	slices.Sort(sizes)
	prev := -1.0
	for _, n := range sizes {
		s := FormatBytes(n)
		got, ok := ParseFormattedNumber(s)
		if !ok {
			t.Fatalf("ParseFormattedNumber(FormatBytes(%d) = %q) is not a number", n, s)
		}
		if diff := math.Abs(got - float64(n)); diff > 0.05*float64(n)+0.5 {
			t.Errorf("FormatBytes(%d) = %q reads back as %v", n, s, got)
		}
		if got < prev {
			t.Errorf("FormatBytes(%d) = %q reads back as %v, below the smaller size's %v", n, s, got, prev)
		}
		prev = got
	}
	for _, n := range []int64{0, 7, -42, 1234, -1234567, math.MaxInt64 / 1024} {
		s := FormatThousands(n)
		if got, ok := ParseFormattedNumber(s); !ok || got != float64(n) {
			t.Errorf("ParseFormattedNumber(FormatThousands(%d) = %q) = %v, %v", n, s, got, ok)
		}
	}
}

func TestFormatClock(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00:00"},
		{-time.Second, "00:00:00"},            // a clock skew must not print "-1:59:59"
		{1500 * time.Millisecond, "00:00:01"}, // truncates: a live timer must not run ahead
		{time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
		{100 * time.Hour, "100:00:00"}, // hours are not clamped to two digits
	}
	for _, c := range cases {
		if got := FormatClock(c.d); got != c.want {
			t.Errorf("FormatClock(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestParseClock(t *testing.T) {
	ok := map[string]time.Duration{
		"00:00:00":         0,
		"01:02:03":         time.Hour + 2*time.Minute + 3*time.Second,
		"100:00:00":        100 * time.Hour,
		"00:00:01.25":      1250 * time.Millisecond,
		"00:00:00.000001":  time.Microsecond,
		"1 day":            24 * time.Hour,
		"2 days 03:04:05":  51*time.Hour + 4*time.Minute + 5*time.Second,
		"-00:00:05":        -5 * time.Second,
		"-1 day 00:00:01":  -(24*time.Hour + time.Second),
		"14:00:01":         14*time.Hour + time.Second,
		"0 days 00:00:00":  0,
		"123456:59:59":     123456*time.Hour + 59*time.Minute + 59*time.Second,
		"99999 days":       99999 * 24 * time.Hour,
		"00:00:00.1234567": 123456700,
		// The largest interval that fits time.Duration, and its negation.
		"99999 days 162070:59:59.999999999":  maxClockDuration,
		"-99999 days 162070:59:59.999999999": -maxClockDuration,
	}
	for in, want := range ok {
		if got, isClock := ParseClock(in); !isClock || got != want {
			t.Errorf("ParseClock(%q) = %v, %v; want %v, true", in, got, isClock, want)
		}
	}
	for _, in := range []string{
		"", "-", "10:30", "1:5:00", "12:60:00", "00:00:60", "00:0a:00", "00:00:00.",
		"00:00:00.1234567890", "a:00:00", "1 week 00:00:00", "day 00:00:00",
		"2026-11-20 14:03:22", "16.0.4135.4", "1,234", "12 MB", "1 mon 2 days",
		" 00:00:01", "00:00:01 ", "+00:00:01", "1234567:00:00", "100000 days", "00:00:00:00",
		// An hour past the largest: it would wrap time.Duration negative.
		"99999 days 162071:00:00", "-99999 days 162071:00:00", "99999 days 999999:00:00",
	} {
		if d, isClock := ParseClock(in); isClock {
			t.Errorf("ParseClock(%q) = %v, want not a duration", in, d)
		}
	}
}

// maxClockDuration is the largest interval ParseClock accepts:
// maxClockHours, then 59:59.999999999.
const maxClockDuration = time.Duration(maxClockHours)*time.Hour + time.Hour - 1

// ParseClock is FormatClock's inverse, so the pair can't drift apart: every
// whole-second duration FormatClock writes reads back exactly.
func TestParseClockRoundTrips(t *testing.T) {
	for _, d := range []time.Duration{0, time.Second, 59 * time.Second, time.Hour - time.Second,
		25*time.Hour + 61*time.Second, 99*time.Hour + 59*time.Minute + 59*time.Second, 1000 * time.Hour} {
		s := FormatClock(d)
		if got, ok := ParseClock(s); !ok || got != d {
			t.Errorf("ParseClock(FormatClock(%v) = %q) = %v, %v", d, s, got, ok)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{40 * 1024 * 1024, "40.0 MB"},
		{5 * 1024 * 1024 * 1024, "5.0 GB"},
		// The unit comes from the rounded value: one byte short of a unit
		// boundary is "1.0 MB", never "1024.0 KB".
		{1<<20 - 1, "1.0 MB"},
		{1<<30 - 1, "1.0 GB"},
		{1<<50 - 1, "1.0 PB"},
		{1 << 50, "1.0 PB"},
		{2 << 50, "2.0 PB"},
		// Past TB: a sparse 2 PiB file once indexed past "KMGT" and panicked.
		{1 << 60, "1.0 EB"},
		{math.MaxInt64, "8.0 EB"},
		{-1, "-1 B"},
	}
	for _, c := range cases {
		if got := FormatBytes(c.n); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestWrapText(t *testing.T) {
	cases := []struct {
		name string
		text string
		w    int
		want []string
	}{
		{"fits on one line", "one two", 20, []string{"one two"}},
		{"greedy fill", "aaa bbb ccc", 7, []string{"aaa bbb", "ccc"}},
		{"exact fit is not a break", "aaa bbb", 7, []string{"aaa bbb"}},
		{"empty text", "   ", 10, []string{""}},
		{"non-positive width passes text through", "anything", 0, []string{"anything"}},
		// A word wider than the line is broken rather than left to overflow.
		// Unbroken it came back as one 12-column line for a 5-column pane,
		// and every caller draws through DrawTextClipped — so the tail was
		// cut off the screen with no ellipsis and no way to reach it.
		{"long word is hard-broken", "abcdefghijkl", 5, []string{"abcde", "fghij", "kl"}},
		{"long word after a short one", "hi abcdefghijkl", 5, []string{"hi", "abcde", "fghij", "kl"}},
		{"long word before a short one", "abcdefghijkl hi", 5, []string{"abcde", "fghij", "kl hi"}},
		// The break lands on grapheme boundaries, not bytes: a wide rune
		// that would straddle the edge moves to the next line whole.
		{"wide runes break on a boundary", "你好吗朋友", 4, []string{"你好", "吗朋", "友"}},
		// One grapheme wider than the whole line still has to make progress,
		// or the split loop never terminates.
		{"grapheme wider than the line", "你好", 1, []string{"你", "好"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := WrapText(c.text, c.w)
			if !slices.Equal(got, c.want) {
				t.Fatalf("WrapText(%q, %d) = %q, want %q", c.text, c.w, got, c.want)
			}
		})
	}
}

// Whatever the input, no line may come back wider than the width asked for —
// that is the whole promise, and the one a caller drawing with
// DrawTextClipped depends on. The single exception is a lone grapheme wider
// than the whole line, which has nowhere narrower to go and must still be
// emitted or the split loop cannot make progress. The non-positive-width
// passthrough is the other documented exception, covered above.
func TestWrapTextNeverExceedsWidth(t *testing.T) {
	texts := []string{
		"a short sentence",
		"C:\\Program_Files\\Microsoft_SQL_Server\\MSSQL16.MSSQLSERVER\\MSSQL\\DATA\\db_log.ldf",
		"mixed 你好吗朋友们 with_a_very_long_unbroken_token_indeed and more words",
		strings.Repeat("x", 200),
		"你好吗朋友们你好吗朋友们",
	}
	for _, text := range texts {
		for w := 1; w <= 40; w++ {
			for _, line := range WrapText(text, w) {
				got := DisplayWidth(line)
				if got <= w {
					continue
				}
				if _, rest := splitGrapheme(line, 0); rest == "" {
					continue // one grapheme, wider than w on its own
				}
				t.Fatalf("WrapText(%q, %d) produced a %d-column line %q", text, w, got, line)
			}
		}
	}
}

// No line comes back empty either — a blank row in a wrapped message reads as
// a paragraph break that isn't there. A word dividing exactly into full lines
// used to leave one on the end.
func TestWrapTextEmitsNoBlankLines(t *testing.T) {
	for _, text := range []string{"abcdefghij", "hi abcdefghij", "你好", "你好吗朋友们"} {
		for w := 1; w <= 12; w++ {
			lines := WrapText(text, w)
			for i, line := range lines {
				if line == "" {
					t.Fatalf("WrapText(%q, %d) = %q — line %d is blank", text, w, lines, i)
				}
			}
		}
	}
}

// Breaking a word must not lose or reorder any of it.
func TestWrapTextPreservesEveryWord(t *testing.T) {
	text := "prefix C:\\a_very_long_unbroken_path_component_here\\file.ldf suffix"
	for w := 1; w <= 30; w++ {
		joined := strings.Join(WrapText(text, w), "")
		want := strings.ReplaceAll(text, " ", "")
		if strings.ReplaceAll(joined, " ", "") != want {
			t.Fatalf("WrapText(text, %d) lost or reordered characters: %q", w, joined)
		}
	}
}

// evRuneStub implements the minimal interface{ Str() string } EvRune expects,
// standing in for a real tcell.EventKey.
type evRuneStub struct{ s string }

func (e evRuneStub) Str() string { return e.s }

func TestEvRune(t *testing.T) {
	cases := []struct {
		s    string
		want rune
	}{
		{"a", 'a'},
		{"", 0},
		{"你", '你'},
	}
	for _, c := range cases {
		if got := EvRune(evRuneStub{c.s}); got != c.want {
			t.Errorf("EvRune(%q) = %q, want %q", c.s, got, c.want)
		}
	}
}

// TestWrapTextLimitDoesNotInventSpaces pins the fold's one subtlety. WrapText
// hard-breaks a token too long for the line, so rejoining the overflow with a
// space — which is what the two call sites did before WrapTextLimit existed —
// splits an unreachable UNC path into two plausible-looking ones.
func TestWrapTextLimitDoesNotInventSpaces(t *testing.T) {
	const path = "bakserverxyz.corp.internal.example.invalid/share/db.bak"
	lines := WrapTextLimit(path+" is unreachable", 20, 2)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	// The clipped line is a prefix of the path continuing from line 1, with
	// nothing inserted at the break.
	joined := strings.TrimSuffix(lines[0]+lines[1], "…")
	if !strings.HasPrefix(path, joined) {
		t.Errorf("folded lines %q + %q = %q, which is not a prefix of %q — a space was injected mid-token",
			lines[0], lines[1], joined, path)
	}
}

// A soft break is still rejoined with the space it was broken at.
func TestWrapTextLimitKeepsSpacesAtSoftBreaks(t *testing.T) {
	lines := WrapTextLimit("alpha beta gamma delta epsilon", 11, 2)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if strings.Contains(lines[1], "gammadelta") {
		t.Errorf("last line = %q, want the words kept apart", lines[1])
	}
}

// TestWrapParagraphsKeepsLineBreaks pins what WrapText deliberately does not:
// each line is wrapped on its own, blank lines survive, any line-break style
// counts, and breaks at either end add no blank line.
func TestWrapParagraphsKeepsLineBreaks(t *testing.T) {
	got := WrapParagraphs("\nalpha beta gamma\r\n\r\ndelta\repsilon\n", 11)
	want := []string{"alpha beta", "gamma", "", "delta", "epsilon"}
	if !slices.Equal(got, want) {
		t.Errorf("WrapParagraphs = %q, want %q", got, want)
	}
	if w := ParagraphsWidth("ab\n\nabcd  efg\n"); w != 8 {
		t.Errorf("ParagraphsWidth = %d, want 8 (widest line, whitespace collapsed)", w)
	}
}

// The cap folds across paragraph breaks; the gap folds away rather than
// leaving a doubled or leading space.
func TestWrapParagraphsLimitFoldsAcrossParagraphs(t *testing.T) {
	got := WrapParagraphsLimit("one\n\ntwo\nthree", 20, 2)
	want := []string{"one", "two three"}
	if !slices.Equal(got, want) {
		t.Errorf("WrapParagraphsLimit = %q, want %q", got, want)
	}
	if got := WrapParagraphsLimit("x", 0, 2); got != nil {
		t.Errorf("w=0: got %q, want nil", got)
	}
}

// TestPadIsExactlyNColumnsForAnyInput pins the contract both functions state:
// exactly n display columns, whatever they are handed. Malformed UTF-8 broke
// it — a trailing partial cluster absorbed the padding space and PadRight
// came back 3 columns wide for n=2.
func TestPadIsExactlyNColumnsForAnyInput(t *testing.T) {
	inputs := []string{"", "a", "ab", "abc", "0\xcc", "\xcc", "\xff\xfe", "é", "日", "日本語", "é", "a\xcc\xcc"}
	for _, s := range inputs {
		for n := 1; n <= 5; n++ {
			if w := DisplayWidth(PadRight(s, n)); w != n {
				t.Errorf("DisplayWidth(PadRight(%q, %d)) = %d, want %d", s, n, w, n)
			}
			if w := DisplayWidth(PadLeft(s, n)); w != n {
				t.Errorf("DisplayWidth(PadLeft(%q, %d)) = %d, want %d", s, n, w, n)
			}
		}
	}
}

// DisplayWidthAtMost must agree with the full measure wherever the full
// measure is within the limit, and report exactly the limit otherwise — a
// grid sizing columns with it would otherwise size them differently from the
// width it then draws. Combining marks, zero-width joiners and emoji are where
// a byte-length shortcut or an early exit could go wrong.
func TestDisplayWidthAtMostMatchesDisplayWidth(t *testing.T) {
	texts := []string{
		"", "a", "abc", "你好吗", "ééé", "👩‍👩‍👧‍👦 family",
		"x\u200by\u200bz", "mixed 你好 ascii", strings.Repeat("ab你", 50),
		strings.Repeat("\u0301", 30) + "abc",
	}
	for _, s := range texts {
		full := DisplayWidth(s)
		for n := -1; n <= full+3; n++ {
			want := max(min(full, n), 0)
			if got := DisplayWidthAtMost(s, n); got != want {
				t.Errorf("DisplayWidthAtMost(%q, %d) = %d, want %d (full width %d)", s, n, got, want, full)
			}
		}
	}
}

// TruncateLine shows CR, LF, CRLF and TAB as one space each, measured as one
// column, so a multi-line cell reads "line1 line2" instead of "line1line2"
// (T67), and clips like Truncate.
func TestTruncateLine(t *testing.T) {
	for _, c := range []struct {
		s    string
		n    int
		want string
	}{
		{"line1\r\nline2", 20, "line1 line2"},
		{"a\nb\rc\td", 20, "a b c d"},
		{"a\n\nb", 20, "a  b"},
		{"abc\r\ndef", 5, "abc …"},
		{"abc\ndef", 7, "abc def"},
		{"abcdef", 4, "abc…"},
		{"世界\n世界", 5, "世界…"},
		{"plain", 10, "plain"},
		{"x", 0, ""},
	} {
		if got := TruncateLine(c.s, c.n); got != c.want {
			t.Errorf("TruncateLine(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
		if got := TruncateLine(c.s, c.n); DisplayWidth(got) > c.n {
			t.Errorf("TruncateLine(%q, %d) = %q is %d columns wide", c.s, c.n, got, DisplayWidth(got))
		}
	}
	// No copy when there is nothing to map.
	s := "no breaks here"
	if got := TruncateLine(s, 40); unsafe.StringData(got) != unsafe.StringData(s) {
		t.Error("a string without breaks was copied")
	}
}

func TestTrimLastGrapheme(t *testing.T) {
	cases := []struct{ s, want string }{
		{"", ""},
		{"a", ""},
		{"ab", "a"},
		{"é", ""},     // precomposed, two bytes
		{"xé", "x"},  // e + combining acute: one cluster
		{"a你", "a"},   // three-byte rune
		{"🇷🇴", ""},    // flag: two regional indicators, one cluster
		{"a👩‍💻", "a"}, // ZWJ sequence
	}
	for _, c := range cases {
		if got := TrimLastGrapheme(c.s); got != c.want {
			t.Errorf("TrimLastGrapheme(%q) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestSafeFileName(t *testing.T) {
	for in, want := range map[string]string{
		`a\b/c:d*e?f"g<h>i|j`: "a_b_c_d_e_f_g_h_i_j",
		"a\x00b\tc\x7f":       "a_b_c_",
		"name. . ":            "name",
		"":                    "_",
		".":                   "_",
		"..":                  "_",
		"...":                 "_",
		"日本語 table":           "日本語 table",
		"CON":                 "_CON",
		"con":                 "_con",
		"nul.tar.gz":          "_nul.tar.gz",
		"AUX .txt":            "_AUX .txt",
		"COM1":                "_COM1",
		"lpt9":                "_lpt9",
		"COM¹":                "_COM¹",
		"PRN ":                "_PRN",
		"CONSOLE":             "CONSOLE",
		"CONOUT$":             "_CONOUT$",
		"conin$.txt":          "_conin$.txt",
		"CONIN$x":             "CONIN$x",
		"COM10":               "COM10",
		"LPT":                 "LPT",
		"xCON":                "xCON",
	} {
		if got := SafeFileName(in); got != want {
			t.Errorf("SafeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A long name is cut to safeFileNameMax bytes on a rune boundary, so two joined
// with an extension ("schema.name.sql") stay inside a 255-byte component, and
// a cut that leaves a trailing dot or space is trimmed like any other.
func TestSafeFileNameCapsLengthOnARuneBoundary(t *testing.T) {
	for _, in := range []string{
		strings.Repeat("a", 300),
		strings.Repeat("日", 100), // 3 bytes each: 120 is a boundary
		"a" + strings.Repeat("日", 100),
		strings.Repeat("a", safeFileNameMax-1) + ". tail",
	} {
		got := SafeFileName(in)
		if len(got) > safeFileNameMax || !utf8.ValidString(got) || !strings.HasPrefix(in, got) {
			t.Errorf("SafeFileName(%d bytes) = %q (%d bytes), want a valid prefix of at most %d", len(in), got, len(got), safeFileNameMax)
		}
		if strings.HasSuffix(got, ".") || strings.HasSuffix(got, " ") {
			t.Errorf("SafeFileName cut %q ends in a dot or space", got)
		}
	}
	if n := 2*safeFileNameMax + len(".sqlplan") + 1; n > 255 {
		t.Errorf("two capped names plus an extension are %d bytes, over 255", n)
	}
}

func TestMatchesAny(t *testing.T) {
	cases := []struct {
		term   string
		fields []string
		want   bool
	}{
		{"", []string{"anything"}, true},
		{"  ", []string{"anything"}, true},
		{"", nil, true},
		{"sel", []string{"SELECT"}, true},
		{" SEL ", []string{"select"}, true},
		{"role", []string{"app_reader", "DATABASE_ROLE"}, true},
		{"zzz", []string{"app_reader", "DATABASE_ROLE"}, false},
		{"x", nil, false},
	}
	for _, c := range cases {
		if got := MatchesAny(c.term, c.fields...); got != c.want {
			t.Errorf("MatchesAny(%q, %q) = %v, want %v", c.term, c.fields, got, c.want)
		}
	}
}
