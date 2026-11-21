package editor

import (
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/radix29/tuikit/core"
)

// ---------------------------------------------------------------------------
// Find and replace for Editor
// ---------------------------------------------------------------------------

// SearchOptions describes one find/replace request. Literal and regexp searches
// share one engine: a literal Query is escaped with regexp.QuoteMeta and compiled
// the same way, so match iteration, whole-word handling and case folding have one
// implementation.
type SearchOptions struct {
	Query     string
	Replace   string
	MatchCase bool
	WholeWord bool
	Regexp    bool

	// InSelection restricts FindNext, ReplaceCurrent and ReplaceAll to the
	// selection active when SetSearch ran (SSMS's "Look in: Selection"): FindNext
	// wraps within it, so the first Replace press finds a match Replace will
	// replace rather than the next one after the selection. The scope follows
	// edits that leave its first and last rows alone; one it cannot follow (an
	// edit on either row, SetText) empties it, so nothing is found or replaced
	// rather than text the user did not select. A plain Find that should cover
	// the whole document leaves this unset.
	InSelection bool
}

// searchMatch is one match, as rune indices into a single logical line. The
// pattern is applied per line, so a match never spans lines, which keeps the
// per-line list usable directly by the drawing path.
type searchMatch struct {
	row      int
	startCol int
	endCol   int
}

// editorSearch is Editor's find/replace state: the compiled pattern, the match
// list derived from it, and the current match. matches is cached against the
// document version the scan ran on; Draw consults it every event, so rescanning
// per Draw would be an O(document) regexp sweep per keystroke.
type editorSearch struct {
	opts SearchOptions
	re   *regexp.Regexp
	// beginAnchored is set when the pattern holds `^` or `\A`, which a search over a
	// line's tail would wrongly satisfy at the tail's start (wholeWordLocs).
	beginAnchored bool

	matches []searchMatch
	// found is set when FindNext selected a match. The current match is then the
	// match the selection covers exactly, resolved per call (currentIndex); an index
	// into matches would go stale, because every edit re-splices the list under it
	// (rescanSpan) and Alt+Up moves the selected match to a row where an index or a
	// stored position names another match, so Replace skipped the match the user
	// selected. The selection is what every edit already carries along.
	found      bool
	scanned    bool
	scanVer    uint64
	scanDocPtr *Document

	// selStart/selEnd bound an InSelection search, captured at SetSearch time
	// because replacing text moves the selection (from the second replacement on,
	// the first invalidated the range). They describe the document at scopeVer and
	// are brought forward by syncScope before anything reads them: captured once
	// and left alone, a Replace on the end row or a line typed above made Replace
	// All filter against positions naming other text, replacing outside the
	// selection or skipping matches inside it.
	selValid                 bool
	selStartRow, selStartCol int
	selEndRow, selEndCol     int
	scopeVer                 uint64
}

// SetSearch compiles opts into the active search and drops any previous match
// state. An invalid regexp is reported as an error and leaves no active search,
// so a half-typed pattern highlights nothing rather than the previous pattern's
// hits. An empty Query clears the search, like ClearSearch.
func (e *Editor) SetSearch(opts SearchOptions) error {
	if opts.Query == "" {
		e.ClearSearch()
		return nil
	}
	pat := opts.Query
	if !opts.Regexp {
		pat = regexp.QuoteMeta(pat)
	}
	// WholeWord is not a `\b` wrapper: RE2's \b is an ASCII word boundary, so
	// `\b@id\b` never matches "x = @id" and `\bcafé\b` never matches at all.
	// appendLineMatches filters on core.IsWordRune instead, the editor's own word
	// definition, so Find, Ctrl+F3 and word motion agree.
	if !opts.MatchCase {
		pat = `(?i)` + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		e.ClearSearch()
		return err
	}
	e.search = editorSearch{opts: opts, re: re, beginAnchored: hasBeginAnchor(pat)}
	if opts.InSelection && e.HasSelection() && !e.selBlock {
		sr, sc, er, ec := e.selectionBounds()
		e.search.selValid = true
		e.search.selStartRow, e.search.selStartCol = sr, sc
		e.search.selEndRow, e.search.selEndCol = er, ec
		e.search.scopeVer = e.doc.Version()
	}
	return nil
}

// syncScope brings an InSelection scope forward to the current document. An edit
// wholly above the scope moves it by the change in line count, one wholly below
// it or strictly between its first and last rows changes nothing but the last
// row's number, and anything else (an edit on the first or last row, which may
// have moved the columns, or one changedSince can't describe) empties it. The
// columns are never guessed: an uncertain scope replaces nothing. replaceMatch,
// the one edit whose column effect is known, adjusts the scope itself.
//
// Called from scanMatches, so every Draw keeps the scope current and
// changedSince's union rarely widens across far-apart edits.
func (e *Editor) syncScope() {
	s := &e.search
	if !s.selValid || s.scopeVer == e.doc.Version() {
		return
	}
	row, oldN, newN, ok := e.doc.changedSince(s.scopeVer)
	s.scopeVer = e.doc.Version()
	d := newN - oldN
	switch {
	case !ok:
		s.selValid = false
	case row+oldN <= s.selStartRow:
		// Above (an insertion at the first row, oldN == 0, puts lines before it).
		s.selStartRow += d
		s.selEndRow += d
	case row > s.selEndRow:
		// Below.
	case row > s.selStartRow && row+oldN <= s.selEndRow:
		// Between the first and last rows: their text is untouched.
		s.selEndRow += d
	default:
		s.selValid = false
	}
}

// scopeAfterReplace moves an InSelection scope for replaceMatch's rewrite of m,
// which syncScope can't do: it sees only that the row changed. extraRows is how
// many rows the replacement added (a line break in it splits the row) and lastLen
// the width in runes of its last row. A position at or past the match's end moves
// with the text after it, which now starts lastLen runes into the last new row
// (after startCol too, when no row was added); rows below shift by extraRows. The
// caller has synced the scope to the version before the rewrite.
func (s *editorSearch) scopeAfterReplace(m searchMatch, extraRows, lastLen int, ver uint64) {
	if !s.selValid {
		return
	}
	newEnd := lastLen
	if extraRows == 0 {
		newEnd += m.startCol
	}
	for _, p := range []struct{ row, col *int }{{&s.selStartRow, &s.selStartCol}, {&s.selEndRow, &s.selEndCol}} {
		switch {
		case *p.row > m.row:
			*p.row += extraRows
		case *p.row < m.row:
		case *p.col >= m.endCol:
			*p.row += extraRows
			*p.col += newEnd - m.endCol
		case *p.col > m.startCol:
			// Inside the replaced text: no position in the new text corresponds.
			s.selValid = false
			return
		}
	}
	s.scopeVer = ver
}

// SetReplacement changes the active search's Replace text and nothing else: the
// compiled pattern, match list, current match and InSelection scope all stay,
// unlike SetSearch. A no-op when no search is active.
func (e *Editor) SetReplacement(s string) {
	if e.search.re != nil {
		e.search.opts.Replace = s
	}
}

// ClearSearch drops the active search, its match list and its highlighting.
func (e *Editor) ClearSearch() { e.search = editorSearch{} }

// HasSearch reports whether a search pattern is active.
func (e *Editor) HasSearch() bool { return e.search.re != nil }

// SearchOpts returns the options the active search was compiled from.
func (e *Editor) SearchOpts() SearchOptions { return e.search.opts }

// scanMatches brings the match list up to date with the document and returns it.
//
// Typing with a search active is the hot path: nothing clears the search when the
// Find dialog closes (F3 must keep working, SSMS parity), so every keystroke for
// the rest of the panel's life lands here through Draw. A full rescan is a regexp
// sweep over every line (123ms on a 20,000-line script), so an edit resumes
// instead, as rewrapSpan does for wrap segments: Document.changedSince names the
// lines the edits since the last scan touched, only those are re-scanned, and the
// matches below them are renumbered by the change in line count. Resuming only on
// a same-line-count edit cost a full rescan per Enter (132 ms at 20,000 lines).
// An edit changedSince can't describe, or a different document, does the full
// scan.
func (e *Editor) scanMatches() []searchMatch {
	s := &e.search
	if s.re == nil {
		return nil
	}
	e.syncScope()
	doc := e.doc
	switch {
	case !s.scanned || s.scanDocPtr != doc:
		s.fullScan(doc)
	case s.scanVer == doc.Version():
		// Nothing has changed since the last scan.
	default:
		if row, oldN, newN, ok := doc.changedSince(s.scanVer); ok {
			s.rescanSpan(doc, row, oldN, newN)
		} else {
			s.fullScan(doc)
		}
	}
	return s.matches
}

// fullScan rebuilds the whole match list from the document.
func (s *editorSearch) fullScan(doc *Document) {
	s.matches = s.matches[:0]
	for row, line := range doc.Lines() {
		s.matches = s.appendLineMatches(s.matches, row, string(line))
	}
	s.stamp(doc)
}

// rescanSpan updates the match list for a change in changedSince's terms: the
// matches on scanned lines [row, row+oldN) are replaced by fresh ones from the
// current lines [row, row+newN), and later matches are renumbered by the change
// in line count. The list stays sorted by row (matchSpansForLine's binary search
// and FindNext's ordering depend on it) because the replacement occupies exactly
// the old span's position.
func (s *editorSearch) rescanSpan(doc *Document, row, oldN, newN int) {
	start, _ := rowRange(s.matches, row)
	end, _ := rowRange(s.matches, row+oldN)
	var fresh []searchMatch
	for li := row; li < row+newN; li++ {
		fresh = s.appendLineMatches(fresh, li, string(doc.Line(li)))
	}
	s.matches = slices.Replace(s.matches, start, end, fresh...)
	if d := newN - oldN; d != 0 {
		for i := start + len(fresh); i < len(s.matches); i++ {
			s.matches[i].row += d
		}
	}
	s.stamp(doc)
}

// stamp records which document and version the match list now describes.
func (s *editorSearch) stamp(doc *Document) {
	s.scanned, s.scanVer, s.scanDocPtr = true, doc.Version(), doc
}

// appendLineMatches appends every match on one line's text to dst.
func (s *editorSearch) appendLineMatches(dst []searchMatch, row int, text string) []searchMatch {
	if text == "" {
		return dst
	}
	// Byte offsets from the regexp engine become rune indices once per line rather
	// than per match: every position Editor works in is a rune index, and a byte
	// offset reaching one lands mid-character on the first non-ASCII line.
	byteToRune := byteRuneIndex(text)
	for _, loc := range s.lineMatchLocs(text, false) {
		start, end := loc[0], loc[1]
		if byteToRune != nil {
			start, end = byteToRune[start], byteToRune[end]
		}
		if start == end {
			// A zero-width match (`^`, `\b`, `x*`) has nothing to select or replace, and Find
			// Next would stall on it forever.
			continue
		}
		dst = append(dst, searchMatch{row: row, startCol: start, endCol: end})
	}
	return dst
}

// lineMatchLocs returns the byte bounds of every match on one line's text, shaped
// like FindStringIndex's result, or FindStringSubmatchIndex's when submatches is
// set (Replace needs the groups; the per-Draw scan doesn't, and asking costs the
// regexp engine its fast path).
func (s *editorSearch) lineMatchLocs(text string, submatches bool) [][]int {
	if !s.opts.WholeWord || s.beginAnchored {
		var locs [][]int
		if submatches {
			locs = s.re.FindAllStringSubmatchIndex(text, -1)
		} else {
			locs = s.re.FindAllStringIndex(text, -1)
		}
		if s.opts.WholeWord {
			// Anchored: no tail search (see wholeWordLocs), so a rejected match can hide an
			// overlapping valid one; `^` binds a match to the line start, so it takes an
			// alternation like `^a|b` to show.
			locs = slices.DeleteFunc(locs, func(loc []int) bool {
				return !isWholeWord(text, loc[0], loc[1])
			})
		}
		return locs
	}
	return s.wholeWordLocs(text, submatches)
}

// wholeWordLocs finds the whole-word matches on text one at a time. A rejected
// candidate must not hide an overlapping valid one, so after a rejection the
// search resumes one rune past the candidate's start rather than at its end, which
// FindAll cannot do.
//
// Resuming searches text[pos:], where the regexp sees pos as the start of text.
// That changes only assertions evaluated at pos: `^`/`\A`, which beginAnchored
// keeps away from this path, and `\b`/`\B`. Those agree with the full line
// whenever the rune before pos is not a word rune; when it is, a match starting at
// pos fails isWholeWord's left check whatever the engine decided, and the search
// moves on.
func (s *editorSearch) wholeWordLocs(text string, submatches bool) [][]int {
	find := s.re.FindStringIndex
	if submatches {
		find = s.re.FindStringSubmatchIndex
	}
	var locs [][]int
	for pos := 0; pos <= len(text); {
		loc := find(text[pos:])
		if loc == nil {
			break
		}
		for i := range loc {
			if loc[i] >= 0 {
				loc[i] += pos
			}
		}
		start, end := loc[0], loc[1]
		if start < end && isWholeWord(text, start, end) {
			locs = append(locs, loc)
			pos = end
			continue
		}
		if start == len(text) {
			break
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		pos = start + size
	}
	return locs
}

// isWholeWord reports whether text[start:end] has no word rune directly before or
// after it. Only outside neighbours count: `@id` in "x = @id" is a whole word
// though `@` is not a word rune, as `#tmp` and `@@ROWCOUNT` are.
func isWholeWord(text string, start, end int) bool {
	if r, _ := utf8.DecodeLastRuneInString(text[:start]); start > 0 && core.IsWordRune(r) {
		return false
	}
	if r, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && core.IsWordRune(r) {
		return false
	}
	return true
}

// hasBeginAnchor reports whether pat, a pattern regexp.Compile accepted, holds
// a start-of-text or start-of-line assertion anywhere.
func hasBeginAnchor(pat string) bool {
	re, err := syntax.Parse(pat, syntax.Perl)
	if err != nil {
		return true // the conservative answer: FindAll over the whole line
	}
	var walk func(*syntax.Regexp) bool
	walk = func(r *syntax.Regexp) bool {
		if r.Op == syntax.OpBeginText || r.Op == syntax.OpBeginLine {
			return true
		}
		return slices.ContainsFunc(r.Sub, walk)
	}
	return walk(re)
}

// rowRange returns the half-open range of row's matches in a row-sorted list,
// [start, start) when the row has none.
func rowRange(matches []searchMatch, row int) (start, end int) {
	start, _ = slices.BinarySearchFunc(matches, row, func(m searchMatch, r int) int {
		return m.row - r
	})
	end = start
	for end < len(matches) && matches[end].row == row {
		end++
	}
	return start, end
}

// byteRuneIndex maps every byte offset of s that starts a rune (plus len(s)) to
// that rune's index. Offsets inside a multi-byte rune stay zero; regexp match
// bounds always land on rune boundaries, so they are never read. Returns nil
// when s is pure ASCII (the identity, the common case in T-SQL), where the caller
// uses the byte offset directly; the map costs an allocation per line per rescan.
func byteRuneIndex(s string) []int {
	if !hasMultiByte(s) {
		return nil
	}
	idx := make([]int, len(s)+1)
	n := 0
	for i := range s {
		idx[i] = n
		n++
	}
	idx[len(s)] = n
	return idx
}

// hasMultiByte reports whether s holds any non-ASCII byte, i.e. whether byte
// offsets and rune indices can differ.
func hasMultiByte(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

// currentIndex returns the index in matches of the current match, or -1 when
// none is: no Find has selected one since the search was set or the last
// ReplaceAll, or the selection no longer covers exactly a match (typed over,
// extended, collapsed by an edit or SetText).
func (e *Editor) currentIndex(matches []searchMatch) int {
	if !e.search.found || !e.HasSelection() || e.selBlock {
		return -1
	}
	sr, sc, er, ec := e.selectionBounds()
	if sr != er {
		return -1
	}
	start, end := rowRange(matches, sr)
	for i := start; i < end; i++ {
		if matches[i].startCol == sc && matches[i].endCol == ec {
			return i
		}
	}
	return -1
}

// MatchCount returns the number of matches in the document for the active search.
func (e *Editor) MatchCount() int { return len(e.scanMatches()) }

// MatchPosition returns the 1-based ordinal of the current match and the total
// count, for a "Match 2 of 7" readout. i is 0 when no match is current.
func (e *Editor) MatchPosition() (i, n int) {
	matches := e.scanMatches()
	return e.currentIndex(matches) + 1, len(matches)
}

// WordAtCursor returns the identifier the caret sits in or next to, or "" on
// whitespace or punctuation, for Ctrl+F3, which needs the word without disturbing
// the selection.
func (e *Editor) WordAtCursor() string {
	line := e.doc.Line(e.cursorRow)
	if len(line) == 0 {
		return ""
	}
	col := core.Clamp(e.cursorCol, 0, len(line))
	// Prefer the word to the left when the caret sits just past its last rune, where
	// a double-click or word-jump leaves it.
	probe := col
	if probe >= len(line) || !core.IsWordRune(line[probe]) {
		if probe > 0 && core.IsWordRune(line[probe-1]) {
			probe--
		} else {
			return ""
		}
	}
	start := probe
	for start > 0 && core.IsWordRune(line[start-1]) {
		start--
	}
	end := probe
	for end < len(line) && core.IsWordRune(line[end]) {
		end++
	}
	return string(line[start:end])
}

// CurrentMatchPos returns the 1-based line and column the current match
// starts at, for a status readout. ok is false when no match is current.
func (e *Editor) CurrentMatchPos() (line, col int, ok bool) {
	matches := e.scanMatches()
	cur := e.currentIndex(matches)
	if cur < 0 {
		return 0, 0, false
	}
	m := matches[cur]
	return m.row + 1, m.startCol + 1, true
}

// FindNext moves to the next match after the cursor (dir >= 0) or the last one
// before it (dir < 0), selects it, and scrolls it into view, wrapping around the
// document (around the scope, under InSelection); it reports whether any match
// was found. The search starts from the
// cursor, not the previous match's index, so Find Next after clicking elsewhere
// continues from where the user is looking.
func (e *Editor) FindNext(dir int) bool {
	matches := e.scanMatches()
	if e.search.opts.InSelection {
		// Only the scope's matches, so the search wraps within it.
		matches = slices.DeleteFunc(slices.Clone(matches), func(m searchMatch) bool { return !e.matchInScope(m) })
	}
	if len(matches) == 0 {
		e.search.found = false
		return false
	}
	idx := -1
	if dir >= 0 {
		// From the cursor's column onward (from its end, with a match selected), so
		// repeated Find Next steps off the match just selected instead of re-selecting it.
		row, col := e.cursorRow, e.cursorCol
		if e.HasSelection() {
			// A selection's start is where the current match begins; search from its end so
			// the match under it is skipped.
			_, _, er, ec := e.selectionBounds()
			row, col = er, ec
		}
		for i, m := range matches {
			if m.row > row || (m.row == row && m.startCol >= col) {
				idx = i
				break
			}
		}
		if idx < 0 {
			idx = 0 // wrap to the top
		}
	} else {
		row, col := e.cursorRow, e.cursorCol
		if e.HasSelection() {
			// Step back from the selection's start, so Find Previous on a selected match
			// moves off it.
			row, col, _, _ = e.selectionBounds()
		}
		for i, m := range slices.Backward(matches) {
			if m.row < row || (m.row == row && m.endCol <= col) {
				idx = i
				break
			}
		}
		if idx < 0 {
			idx = len(matches) - 1 // wrap to the bottom
		}
	}
	e.search.found = true
	e.selectMatch(matches[idx])
	return true
}

// FindAtCursor selects the match the caret is in, or the one it sits just
// past, and otherwise behaves as FindNext(1); it reports whether any match was
// found. It is the first step of a find-word-at-cursor key (Ctrl+F3), whose
// search is the word under the caret: FindNext searches from the caret (from
// a selection's end), so it skipped that very word and selected the next
// occurrence. A match containing the caret wins over one ending at it, as
// WordAtCursor prefers the word to the caret's right.
func (e *Editor) FindAtCursor() bool {
	matches := e.scanMatches()
	if e.search.opts.InSelection {
		matches = slices.DeleteFunc(slices.Clone(matches), func(m searchMatch) bool { return !e.matchInScope(m) })
	}
	row, col := e.cursorRow, e.cursorCol
	if e.HasSelection() {
		row, col, _, _ = e.selectionBounds()
	}
	pick := -1
	for i, m := range matches {
		if m.row != row || m.startCol > col || m.endCol < col {
			continue
		}
		if m.endCol > col { // contains the caret
			pick = i
			break
		}
		if pick < 0 { // ends at the caret; keep looking for one containing it
			pick = i
		}
	}
	if pick < 0 {
		return e.FindNext(1)
	}
	e.search.found = true
	e.selectMatch(matches[pick])
	return true
}

// selectMatch makes m the editor's selection, cursor at its end, and scrolls it
// into view.
func (e *Editor) selectMatch(m searchMatch) {
	e.selecting = true
	e.selBlock = false
	e.selAnchorRow, e.selAnchorCol = m.row, m.startCol
	e.cursorRow, e.cursorCol = m.row, m.endCol
	e.clampCursor()
	e.desiredCol = e.cursorDisplayCol()
	// ensureCursorVisible scrolls sideways too, which a match far along a long line
	// needs: inside the viewport vertically, off it horizontally.
	e.ensureCursorVisible()
}

// ReplaceCurrent replaces the selected match with the active search's Replace
// text and advances to the next, reporting whether it replaced anything. It does
// nothing unless the selection is exactly a match (the SSMS/VS rule: on a fresh
// dialog Replace finds first and replaces on the second press). For a regexp
// search $1 group references are expanded (replacementFor); a literal replacement
// is inserted as-is.
func (e *Editor) ReplaceCurrent() bool {
	if e.readOnly || e.search.re == nil {
		return false
	}
	matches := e.scanMatches()
	cur := e.currentIndex(matches)
	if cur < 0 || !e.matchInScope(matches[cur]) {
		return false
	}
	m := matches[cur]
	repl, ok := e.search.replacementFor(m, e.search.submatchesOf(e.doc.Line(m.row)))
	if !ok {
		return false
	}
	// The step is that one row (a replacement with line breaks grows the span from
	// inside, as undo allows): Replace/F3 down a large script is a held key.
	e.pushUndoSpan(m.row, m.row+1)
	e.replaceMatch(m, repl)
	e.FindNext(1)
	return true
}

// lineSubmatches is one line's text with its regexp matches, submatches included,
// for expanding replacement templates.
type lineSubmatches struct {
	text       string
	locs       [][]int // FindStringSubmatchIndex-shaped, byte offsets into text
	byteToRune []int   // byteRuneIndex(text)
}

// submatchesOf scans line for replacementFor. Only a regexp search needs it; a
// literal one returns the zero value without scanning.
func (s *editorSearch) submatchesOf(line []rune) lineSubmatches {
	if !s.opts.Regexp {
		return lineSubmatches{}
	}
	text := string(line)
	return lineSubmatches{text: text, locs: s.lineMatchLocs(text, true), byteToRune: byteRuneIndex(text)}
}

// replacementFor returns the text that replaces m. A regexp template is expanded
// against the match in its line, never by re-running the pattern on the matched
// text cut out of it: there `\B`, `\b`, `^` and `$` see the cut's edges (`\Bing`
// fails on "ing" alone, so Replace would count a replacement and change nothing),
// and an alternative matching again inside the cut would be substituted twice.
// line must be the row's text before any replacement on it: Replace All rewrites a
// row right to left, and `$` or a trailing `\b` would see the already-replaced
// tail. ok is false when m is not a match of line, so nothing is replaced.
func (s *editorSearch) replacementFor(m searchMatch, line lineSubmatches) (repl string, ok bool) {
	if !s.opts.Regexp {
		return s.opts.Replace, true
	}
	for _, loc := range line.locs {
		start, end := loc[0], loc[1]
		if line.byteToRune != nil {
			start, end = line.byteToRune[start], line.byteToRune[end]
		}
		if start == m.startCol && end == m.endCol {
			return string(s.re.ExpandString(nil, s.opts.Replace, line.text, loc)), true
		}
	}
	return "", false
}

// replaceMatch substitutes repl for m's text in place. A line break in repl
// (\n, \r\n or \r) splits the row, as typing Enter would: the first piece joins the
// row's head, the last its tail, and the pieces between become rows of their own.
// The caller owns the undo step, whose span must cover m's row: ReplaceAll pushes
// one for the whole run, so this must not push its own or Replace All would take
// one Ctrl+Z per occurrence.
func (e *Editor) replaceMatch(m searchMatch, repl string) {
	line := e.doc.Line(m.row)
	pieces := strings.Split(lineBreaks.Replace(e.expandTabs(repl)), "\n")
	head, tail := line[:m.startCol], line[m.endCol:]

	rows := make([][]rune, len(pieces))
	for i, p := range pieces {
		rows[i] = []rune(p)
	}
	last := len(rows) - 1
	lastLen := len(rows[last])
	rows[0] = slices.Concat(head, rows[0])
	rows[last] = slices.Concat(rows[last], tail)

	e.syncScope()
	if last == 0 {
		e.doc.setLine(m.row, rows[0])
	} else {
		e.doc.replaceRange(m.row, 1, rows)
	}
	e.search.scopeAfterReplace(m, last, lastLen, e.doc.Version())

	e.selecting = false
	e.cursorRow, e.cursorCol = m.row+last, lastLen
	if last == 0 {
		e.cursorCol += m.startCol
	}
	e.clampCursor()
}

// lineBreaks folds every line-break convention to \n.
var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// ReplaceAll replaces every match in the document (under InSelection with a
// selection active at SetSearch time, every match inside it) and returns how many
// it replaced. The whole run is one undo step, and each line is rewritten
// right-to-left so replacing one match doesn't shift later offsets on that line.
func (e *Editor) ReplaceAll() int {
	if e.readOnly || e.search.re == nil {
		return 0
	}
	matches := e.scanMatches()
	targets := make([]searchMatch, 0, len(matches))
	for _, m := range matches {
		if e.matchInScope(m) {
			targets = append(targets, m)
		}
	}
	// Every replacement is worked out before the first is made, so each expands
	// against its row as it was (replacementFor).
	repls := make([]string, 0, len(targets))
	lineRow, line := -1, lineSubmatches{}
	for _, m := range targets {
		if m.row != lineRow {
			lineRow, line = m.row, e.search.submatchesOf(e.doc.Line(m.row))
		}
		if repl, ok := e.search.replacementFor(m, line); ok {
			targets[len(repls)] = m
			repls = append(repls, repl)
		}
	}
	targets = targets[:len(repls)]
	if len(targets) == 0 {
		return 0
	}
	// targets is sorted by row (scanMatches' order, filtered), and replaceMatch
	// rewrites only its own row, adding rows below it at most, which the
	// right-to-left walk never revisits.
	e.pushUndoSpan(targets[0].row, targets[len(targets)-1].row+1)
	for i, target := range slices.Backward(targets) {
		e.replaceMatch(target, repls[i])
	}
	e.search.found = false
	e.selecting = false
	e.clampCursor()
	e.ensureCursorVisible()
	return len(targets)
}

// matchInScope reports whether m falls within an InSelection run's captured
// range; always true when the search isn't scoped to a selection. m must come
// from scanMatches, which has synced the scope to the same version.
func (e *Editor) matchInScope(m searchMatch) bool {
	s := &e.search
	if !s.opts.InSelection {
		return true
	}
	if !s.selValid {
		// InSelection was asked for with nothing selected: replacing the whole document
		// would be the opposite of what was asked.
		return false
	}
	if m.row < s.selStartRow || m.row > s.selEndRow {
		return false
	}
	if m.row == s.selStartRow && m.startCol < s.selStartCol {
		return false
	}
	if m.row == s.selEndRow && m.endCol > s.selEndCol {
		return false
	}
	return true
}

// matchSpansForLine returns every match on row, once per drawn row. The list is
// sorted by row, so the row's slice is found by binary search rather than by
// walking a list that can hold a hit per line. The result aliases the cached list
// and must not be retained or modified. The current match is included like any
// other: it is also the selection, and the selection style wins in styleForRune.
func (e *Editor) matchSpansForLine(row int) []searchMatch {
	matches := e.scanMatches()
	if len(matches) == 0 {
		return nil
	}
	start, end := rowRange(matches, row)
	if start == end {
		return nil
	}
	return matches[start:end]
}

// ensureColumnVisible scrolls horizontally so the cursor's display column is
// inside the content area: the horizontal half of ensureCursorVisible (its only
// caller), separate only to keep the vertical arithmetic readable.
func (e *Editor) ensureColumnVisible() {
	if e.wrapMode {
		return
	}
	contentW := e.rect.W - e.gutterWidth()
	if contentW <= 0 {
		return
	}
	// In display columns: a line of wide characters scrolls twice as far per caret
	// step as an ASCII one, as the eye expects.
	col := e.cursorDisplayCol()
	if col < e.scrollCol {
		e.scrollCol = col
	} else if col >= e.scrollCol+contentW {
		e.scrollCol = col - contentW + 1
	}
	e.scrollCol = max(0, e.scrollCol)
}
