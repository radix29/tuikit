package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/clipperhouse/displaywidth"
)

// ---------------------------------------------------------------------------
// String helpers
// ---------------------------------------------------------------------------

// DisplayWidth returns the number of terminal columns s occupies, summing
// grapheme cluster widths (not bytes or runes). Other tuikit packages use this
// rather than importing displaywidth, keeping that dependency in core.
func DisplayWidth(s string) int {
	return displaywidth.String(s)
}

// DisplayWidthAtMost is min(DisplayWidth(s), n), stopping once the width
// reaches n: for a caller that only needs to know whether s fits and may be
// handed a multi-megabyte cell (varchar(max), XML) it would otherwise measure
// in full. n <= 0 returns 0.
func DisplayWidthAtMost(s string, n int) int {
	if n <= 0 {
		return 0
	}
	// No grapheme is wider than its byte length, so a string this short is under
	// the limit, and the full measure's printable-ASCII fast path is cheaper.
	if len(s) <= n {
		return DisplayWidth(s)
	}
	width := 0
	g := displaywidth.StringGraphemes(s)
	for g.Next() {
		if width += g.Width(); width >= n {
			return n
		}
	}
	return width
}

// Truncate clips s to at most n display columns, appending "…" if clipped.
// Works on display width, not rune count, so wide CJK characters and multi-rune
// grapheme clusters are handled correctly.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	// One pass: cut remembers where the string must end to leave a column for the
	// ellipsis, while width keeps running so a string that fits is returned whole.
	budget := n - 1 // reserve one column for the ellipsis
	var pos, cut, width int
	haveCut := false
	g := displaywidth.StringGraphemes(s)
	for g.Next() {
		gw := g.Width()
		if !haveCut && width+gw > budget {
			cut, haveCut = pos, true
		}
		width += gw
		if width > n {
			return s[:cut] + "…"
		}
		pos += len(g.Value())
	}
	return s
}

// TruncateLeft is Truncate cutting the front instead: it keeps the tail of s
// and puts "…" before it, for a path, URL or breadcrumb whose end is the part
// that tells two values apart. Grapheme clusters are never split; a wide one
// straddling the budget is dropped whole.
func TruncateLeft(s string, n int) string {
	if n <= 0 {
		return ""
	}
	// The grapheme iterator only runs forward, so record each cluster's start
	// and width, then spend the budget from the end.
	var starts, widths []int
	pos, width := 0, 0
	g := displaywidth.StringGraphemes(s)
	for g.Next() {
		starts = append(starts, pos)
		widths = append(widths, g.Width())
		width += g.Width()
		pos += len(g.Value())
	}
	if width <= n {
		return s
	}
	budget, cut := n-1, len(s) // reserve one column for the ellipsis
	for i := len(starts) - 1; i >= 0 && widths[i] <= budget; i-- {
		budget -= widths[i]
		cut = starts[i]
	}
	return "…" + s[cut:]
}

// TruncateLine is Truncate for a one-row rendering of text that may hold line
// breaks or tabs (a grid cell). CR, LF, CRLF and TAB each render as one space:
// they measure 0 columns, so otherwise the words either side run together. A
// string without them is not copied.
func TruncateLine(s string, n int) string {
	if n <= 0 {
		return ""
	}
	// buf stays nil until the first break is mapped; until then output is s[:pos].
	var buf []byte
	budget := n - 1 // reserve one column for the ellipsis
	var pos, cut, width int
	haveCut := false
	g := displaywidth.StringGraphemes(s)
	for g.Next() {
		v, gw := g.Value(), g.Width()
		brk := v == "\r\n" || v == "\n" || v == "\r" || v == "\t"
		if brk {
			gw = 1
		}
		if !haveCut && width+gw > budget {
			cut, haveCut = pos, true
		}
		width += gw
		if width > n {
			if buf == nil {
				return s[:cut] + "…"
			}
			return string(buf[:cut]) + "…"
		}
		if brk && buf == nil {
			buf = append(make([]byte, 0, pos+n), s[:pos]...)
		}
		switch {
		case buf == nil:
			pos += len(v)
		case brk:
			buf = append(buf, ' ')
			pos = len(buf)
		default:
			buf = append(buf, v...)
			pos = len(buf)
		}
	}
	if buf == nil {
		return s
	}
	return string(buf)
}

// WrapText greedily word-wraps text to at most w display columns per line.
//
// A word too wide for a line is hard-broken: callers draw through
// DrawTextClipped, so an unbroken token (stack frame, path, thumbprint) would be
// cut at the pane edge with no ellipsis and no way to reach the rest.
//
// Whitespace runs, including leading indentation, are not preserved
// (strings.Fields reflows the paragraph); a caller needing them must keep them.
func WrapText(text string, w int) []string {
	lines, _ := wrapLines(text, w)
	return lines
}

// WrapTextLimit is WrapText capped at maxLines: overflow is folded into the
// last line and clipped there with an ellipsis. Dropping surplus lines would let
// a truncated message (e.g. a SQL Server error) read as complete. A w or
// maxLines of zero or less returns nil.
//
// The fold re-joins with a space only where the wrap broke at one: gluing the
// halves of a hard-broken word with a space turns one unreachable path into two
// plausible ones.
func WrapTextLimit(text string, w, maxLines int) []string {
	if w <= 0 || maxLines <= 0 {
		return nil
	}
	lines, hardBreak := wrapLines(text, w)
	return foldOverflow(lines, hardBreak, w, maxLines)
}

// WrapParagraphs is WrapText that keeps line breaks: each line (split on \n,
// \r\n or a lone \r) is a paragraph wrapped on its own and an empty one stays a
// blank line. Breaks at either end are dropped, so a trailing "\n" gains no
// blank line.
func WrapParagraphs(text string, w int) []string {
	lines, _ := wrapParagraphLines(text, w)
	return lines
}

// WrapParagraphsLimit is WrapParagraphs capped at maxLines as WrapTextLimit caps
// WrapText: overflow, paragraph breaks folded to a space, goes into the last
// line, clipped with an ellipsis. A w or maxLines of zero or less returns nil.
func WrapParagraphsLimit(text string, w, maxLines int) []string {
	if w <= 0 || maxLines <= 0 {
		return nil
	}
	lines, hardBreak := wrapParagraphLines(text, w)
	return foldOverflow(lines, hardBreak, w, maxLines)
}

// ParagraphsWidth is the display width of text's widest line as WrapParagraphs
// draws it (whitespace runs collapsed).
func ParagraphsWidth(text string) int {
	width := 0
	for _, para := range splitParagraphs(text) {
		width = max(width, DisplayWidth(strings.Join(strings.Fields(para), " ")))
	}
	return width
}

// splitParagraphs is text's lines, with \r\n and a lone \r read as \n and
// surrounding whitespace (line breaks included) trimmed first.
func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(strings.TrimSpace(text), "\n")
}

// wrapParagraphLines is wrapLines run per paragraph and concatenated. The break
// after a paragraph's last line is soft, so a fold joins across it with a space.
func wrapParagraphLines(text string, w int) (lines []string, hardBreak []bool) {
	for _, para := range splitParagraphs(text) {
		l, hb := wrapLines(para, w)
		lines, hardBreak = append(lines, l...), append(hardBreak, hb...)
	}
	return lines, hardBreak
}

// foldOverflow caps lines at maxLines, folding the surplus into the last line
// kept and clipping it with an ellipsis; hardBreak says, per line, whether the
// break after it fell inside a word (rejoined without a space). Blank lines are
// dropped from the fold.
func foldOverflow(lines []string, hardBreak []bool, w, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}
	var rest strings.Builder
	rest.WriteString(lines[maxLines-1])
	for i := maxLines; i < len(lines); i++ {
		// A blank line (paragraph gap) contributes nothing, not a space: "a\n\nb" must
		// fold to "a b", not "a  b" or " b".
		if lines[i] == "" {
			continue
		}
		if !hardBreak[i-1] && rest.Len() > 0 {
			rest.WriteByte(' ')
		}
		rest.WriteString(lines[i])
	}
	return append(lines[:maxLines-1], Truncate(rest.String(), w))
}

// wrapLines is WrapText plus, per line, whether the break after it fell inside
// a word rather than at a space, so WrapTextLimit can rejoin the overflow
// without inventing spaces. The last entry describes no break and is false.
func wrapLines(text string, w int) (lines []string, hardBreak []bool) {
	if w <= 0 {
		return []string{text}, []bool{false}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}, []bool{false}
	}
	lines, hardBreak = make([]string, 0, 4), make([]bool, 0, 4)
	// Widths are carried, never re-measured: re-measuring the remainder per hard
	// break made one unbroken n-byte token O(n^2/w) (a 64 KB base64 blob took 33 ms,
	// a 48 KB CJK run 210 ms, on the UI goroutine). A width splits exactly at a
	// grapheme boundary, the only place splitGraphemeWidth cuts.
	cur, curW := "", 0
	for _, word := range words {
		wordW := DisplayWidth(word)
		if cur != "" {
			if curW+1+wordW <= w {
				cur += " " + word
				curW += 1 + wordW
				continue
			}
			lines, hardBreak = append(lines, cur), append(hardBreak, false)
		}
		// word now starts a fresh line. One that doesn't fit an empty line is split
		// until the rest does; splitGraphemeWidth always takes at least one grapheme,
		// so this terminates even for a grapheme wider than w.
		for wordW > w {
			var head string
			var headW int
			head, word, headW = splitGraphemeWidth(word, w)
			wordW -= headW
			lines, hardBreak = append(lines, head), append(hardBreak, true)
		}
		cur, curW = word, wordW
	}
	// cur is empty when the last word divided exactly into full lines; appending it
	// would add a blank line, but the len check keeps "always at least one line".
	if cur != "" || len(lines) == 0 {
		lines, hardBreak = append(lines, cur), append(hardBreak, false)
	}
	return lines, hardBreak
}

// splitGrapheme cuts s at the last grapheme boundary that keeps the head within
// n display columns. At least one grapheme always moves into the head, even one
// wider than n, or WrapText's loop would never end.
func splitGrapheme(s string, n int) (head, rest string) {
	head, rest, _ = splitGraphemeWidth(s, n)
	return head, rest
}

// splitGraphemeWidth is splitGrapheme also returning the head's display width,
// so repeated splitting can track the remainder's width without re-measuring.
func splitGraphemeWidth(s string, n int) (head, rest string, headWidth int) {
	end, width := 0, 0
	g := displaywidth.StringGraphemes(s)
	for g.Next() {
		gw := g.Width()
		if end > 0 && width+gw > n {
			break
		}
		end += len(g.Value())
		width += gw
	}
	return s[:end], s[end:], width
}

// TrimLastGrapheme returns s without its last grapheme cluster, what one
// Backspace removes. Cutting a byte or rune would leave half a multi-byte
// character or a dangling combining mark.
func TrimLastGrapheme(s string) string {
	last := 0
	g := displaywidth.StringGraphemes(s)
	for end := 0; g.Next(); end += len(g.Value()) {
		last = end
	}
	return s[:last]
}

// CenterOffset returns the left padding needed to center content of width
// contentW within a field of width fieldW, clamped to 0 if contentW >= fieldW.
func CenterOffset(fieldW, contentW int) int {
	if contentW >= fieldW {
		return 0
	}
	return (fieldW - contentW) / 2
}

// PadRight pads s to exactly n display columns with trailing spaces. If s is
// already n columns or wider, it is truncated to n (without an ellipsis) so the
// result always occupies exactly n columns.
func PadRight(s string, n int) string { return padSpaces(s, n, false) }

// PadLeft pads s to exactly n display columns with leading spaces, for a
// right-aligned fixed-width column. If s is already n columns or wider, it is
// truncated to n (without an ellipsis) so the result occupies exactly n columns.
func PadLeft(s string, n int) string { return padSpaces(s, n, true) }

// padSpaces is PadRight and PadLeft's shared body; left picks the padding side.
// Both promise *exactly* n display columns, which fixed-width grids depend on.
func padSpaces(s string, n int, left bool) string {
	if n <= 0 {
		return ""
	}
	// Padding is concatenation, and a cluster can absorb following bytes, so a
	// string ending mid-cluster comes back wider than the sum of its parts:
	// PadRight("0\xcc", 2) measured 1 column, then 3 after a space. Only invalid
	// UTF-8 can end mid-cluster; replacing it keeps "exactly n" true.
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	w := DisplayWidth(s)
	switch {
	case w == n:
		return s
	case w < n:
		if left {
			return strings.Repeat(" ", n-w) + s
		}
		return s + strings.Repeat(" ", n-w)
	}
	// Hard-clip to n columns without an ellipsis, for fixed-width cells.
	var sb strings.Builder
	width := 0
	g := displaywidth.StringGraphemes(s)
	for g.Next() {
		gw := g.Width()
		if width+gw > n {
			break
		}
		sb.WriteString(g.Value())
		width += gw
	}
	// Pad the remainder when a wide grapheme straddling the limit didn't fit.
	return sb.String() + strings.Repeat(" ", n-width)
}

// FormatThousands renders n in base 10 with "," every three digits, e.g.
// 1234567 -> "1,234,567".
//
// The digits come from strconv: negating n is a no-op at math.MinInt64, where a
// hand-rolled loop produced no digits ("--").
func FormatThousands(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var sb strings.Builder
	sb.Grow(len(sign) + len(digits) + (len(digits)-1)/3)
	sb.WriteString(sign)
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte(digits[i])
	}
	return sb.String()
}

// FormatBytes renders n bytes with a binary unit and one decimal ("5.8 GB"),
// or as "%d B" below 1 KiB.
//
// The units run to EB because an int64 tops out just below 8 EiB, and a file
// that large is real: a sparse file takes no disk space, so `truncate -s 2P`
// makes one in an instant. With "KMGT" alone, 1 PiB indexed past the end and
// panicked inside the file dialog's Draw. The unit is chosen from the rounded
// value, so 1 MiB − 1 reads "1.0 MB", not "1024.0 KB". A negative size (a
// FileSystem reporting garbage) is left as it is and prints e.g. "-1 B".
func FormatBytes(n int64) string {
	const unit = 1024
	const units = "KMGTPE"
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	v, exp := float64(n)/unit, 0
	for math.Round(v*10)/10 >= unit && exp < len(units)-1 {
		v /= unit
		exp++
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + " " + units[exp:exp+1] + "B"
}

// FormatClock renders a duration as "HH:MM:SS", hours unclamped ("100:00:00").
// It is the one clock-style duration renderer, so a duration that took an hour
// and two minutes reads the same in a progress view, a running timer and a
// grid cell.
//
// Seconds truncate rather than round, which is what a running clock needs: a
// live timer must never show a second that hasn't elapsed. A negative duration
// (clock skew between two timestamps) reads as zero, never "-1:59:59".
func FormatClock(d time.Duration) string {
	sec := int(max(0, d) / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec/60%60, sec%60)
}

// ParseClock reads a cell rendered as a clock-style duration, the inverse of
// FormatClock ("01:02:03", "100:00:00"), and also the forms a database prints
// for an interval: a fraction of a second ("00:00:01.25"), a day count in
// front ("2 days 03:04:05", or "1 day" alone) and a leading minus
// ("-00:00:05"). At most 99999 days and 999999 hours, which together stay
// inside time.Duration's ~292 years. Minutes and seconds are exactly two digits below 60, so
// "1:5:00" and "12:60:00" are not durations, and neither is a two-part
// "10:30" (minutes:seconds or hours:minutes — it can't tell). A time of day
// ("14:00:01") reads as the duration since midnight, which sorts the same.
// s is expected trimmed.
func ParseClock(s string) (time.Duration, bool) {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var d time.Duration
	if n, rest, found := strings.Cut(s, " "); found {
		unit, clock, _ := strings.Cut(rest, " ")
		days, ok := parseDigits(n, 5)
		if !ok || (unit != "day" && unit != "days") {
			return 0, false
		}
		d, s = time.Duration(days)*24*time.Hour, clock
		if s == "" {
			return signed(d, neg), true
		}
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	secs, frac, hasFrac := strings.Cut(parts[2], ".")
	h, okH := parseDigits(parts[0], 6)
	m, okM := parseDigits(parts[1], 2)
	sec, okS := parseDigits(secs, 2)
	if !okH || !okM || !okS || len(parts[1]) != 2 || len(secs) != 2 || m >= 60 || sec >= 60 {
		return 0, false
	}
	d += time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second
	if hasFrac {
		ns, ok := parseDigits(frac, 9)
		if !ok {
			return 0, false
		}
		for range 9 - len(frac) {
			ns *= 10
		}
		d += time.Duration(ns)
	}
	return signed(d, neg), true
}

// parseDigits reads s as 1 to maxLen ASCII digits.
func parseDigits(s string, maxLen int) (int64, bool) {
	if s == "" || len(s) > maxLen {
		return 0, false
	}
	var n int64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int64(s[i]-'0')
	}
	return n, true
}

func signed(d time.Duration, neg bool) time.Duration {
	if neg {
		return -d
	}
	return d
}

// ParseFormattedNumber reads a cell rendered from a number, the inverse of
// FormatThousands ("1,234") and FormatBytes ("4.0 KB"), and also a percentage
// ("45%") or a number followed by a short unit ("250 ms", "12 rows"). Anything
// else (a name, a date, a version such as 16.0.4135.4) is not a number. s is
// expected trimmed.
//
// A byte unit (B through EB, the ones FormatBytes writes) scales the value to
// bytes: FormatBytes picks its unit per value, so one column mixes "512 B",
// "4.0 KB" and "1.2 MB", and comparing the bare mantissas put 4.0 KB before
// 512 B. Other units are only stripped.
//
// strconv.ParseFloat alone is not the test: it also takes "Inf", "NaN" and hex
// floats, which in a cell are names.
func ParseFormattedNumber(s string) (float64, bool) {
	scale := 1.0
	if i := strings.LastIndexByte(s, ' '); i > 0 && isUnitSuffix(s[i+1:]) {
		if f, ok := byteUnitScale[s[i+1:]]; ok {
			scale = f
		}
		s = s[:i]
	}
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || !strings.ContainsAny(s[:1], "0123456789+-.") ||
		strings.ContainsAny(s, "xXpP_") {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	return n * scale, err == nil
}

// byteUnitScale is the factor to bytes of each binary unit FormatBytes writes.
var byteUnitScale = map[string]float64{
	"B": 1, "KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30,
	"TB": 1 << 40, "PB": 1 << 50, "EB": 1 << 60,
}

// isUnitSuffix reports whether s reads as a short unit after a number (MB, KB,
// ms, rows).
func isUnitSuffix(s string) bool {
	if s == "" || len(s) > 4 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// EvRune extracts the first rune from a tcell v3 EventKey (via Str()), for
// matching a key; text insertion uses EvText.
func EvRune(ev interface{ Str() string }) rune {
	for _, r := range ev.Str() {
		return r
	}
	return 0
}

// EvText is every rune a KeyRune event carries: Str is one key or a composed
// sequence (IME commit, ZWJ emoji, base letter plus combining marks), so
// inserting only EvRune dropped all but the first.
func EvText(ev interface{ Str() string }) []rune {
	return []rune(ev.Str())
}

// SafeFileName makes s usable as one path component on every OS: each of
// `\ / : * ? " < > |` and every control character becomes "_", the result is
// cut to safeFileNameMax bytes on a rune boundary, trailing dots and spaces
// (which Windows drops) are trimmed, a Windows device name (CON, NUL, COM1...,
// reserved whatever extension follows) gets a "_" prefix, and an empty, "." or
// ".." result is "_". The Windows rules apply everywhere so a name minted on
// Linux still opens on Windows.
//
// The cap is under half the 255-byte limit most file systems share because
// callers join two results and an extension ("schema.name.sql"); a cut is safe
// there only if both halves fit.
func SafeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
	if len(s) > safeFileNameMax {
		cut := safeFileNameMax
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
	}
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "_"
	}
	if isWindowsDeviceName(s) {
		return "_" + s
	}
	return s
}

const safeFileNameMax = 120

// isWindowsDeviceName reports whether Windows reads name as a device: its stem
// (up to the first dot, trailing spaces ignored) is CON, PRN, AUX, NUL, or COM
// or LPT followed by 0-9 or a superscript 1-3, in any case.
func isWindowsDeviceName(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if rest, ok := strings.CutPrefix(stem, "COM"); ok {
		stem = rest
	} else if rest, ok := strings.CutPrefix(stem, "LPT"); ok {
		stem = rest
	} else {
		return false
	}
	return len(stem) == 1 && stem[0] >= '0' && stem[0] <= '9' || stem == "¹" || stem == "²" || stem == "³"
}

// MatchesAny reports whether any of fields contains term, case-insensitively,
// with term's surrounding spaces ignored. An empty or blank term matches
// everything, so a filter box starts out showing the whole list.
func MatchesAny(term string, fields ...string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), term) {
			return true
		}
	}
	return false
}
