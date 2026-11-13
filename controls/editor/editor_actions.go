package editor

import (
	"slices"
	"strings"
	"unicode"

	"github.com/radix29/tuikit/core"
)

// ---------------------------------------------------------------------------
// Notepad++/Scintilla-style line and block actions for Editor. Every exported
// method here is self-contained (pushes its own undo step, leaves the cursor
// clamped and visible) because each is invoked from Editor.HandleKey and
// directly from a Menu action closure, which bypasses HandleKey.
// ---------------------------------------------------------------------------

// affectedLineRange returns the row range a line/block action applies to: the
// selection's row span if any (selectionBounds' rows are min/max regardless of
// anchor vs. cursor, right for linear and block modes), else the cursor's line.
//
// A linear selection that ends at column 0 of a later row leaves that row out:
// Shift+Down from a line start, or a drag to the start of a line, selects up to
// the previous line's break and none of the row it stops on, so acting on that row
// would comment, indent, move or delete a line the user never selected (Visual
// Studio, SSMS and Notepad++ all exclude it).
func (e *Editor) affectedLineRange() (startRow, endRow int) {
	if e.HasSelection() {
		sr, _, er, ec := e.selectionBounds()
		if !e.selBlock && er > sr && ec == 0 {
			er--
		}
		return sr, er
	}
	return e.cursorRow, e.cursorRow
}

// SelectAll selects the entire buffer as a fresh linear selection.
func (e *Editor) SelectAll() {
	e.selecting = true
	e.selBlock = false
	e.selAnchorRow, e.selAnchorCol = 0, 0
	e.cursorRow = e.doc.Len() - 1
	e.cursorCol = len(e.doc.Line(e.cursorRow))
}

// SelectRange selects the stream from (anchorRow, anchorCol) to (row, col),
// leaving the cursor at the second end and scrolled into view: what a host's
// "select the statement at the cursor" command sets. Both ends are clamped to
// the buffer.
func (e *Editor) SelectRange(anchorRow, anchorCol, row, col int) {
	e.selecting = true
	e.selBlock = false
	e.selAnchorRow = core.Clamp(anchorRow, 0, e.doc.Len()-1)
	e.selAnchorCol = core.Clamp(anchorCol, 0, len(e.doc.Line(e.selAnchorRow)))
	e.cursorRow, e.cursorCol = row, col
	e.clampCursor()
	e.desiredCol = e.cursorDisplayCol()
	e.ensureCursorVisible()
}

// Undo and Redo expose the undo/redo stack to callers outside this package (the
// Edit menu). They finish as Ctrl+Z/Ctrl+Y do: goal column re-anchored and the
// caret scrolled into view, which the step may have moved far from the viewport.
func (e *Editor) Undo() { e.undo(); e.afterHistoryStep() }
func (e *Editor) Redo() { e.redo(); e.afterHistoryStep() }

func (e *Editor) afterHistoryStep() {
	e.desiredCol = e.cursorDisplayCol()
	e.ensureCursorVisible()
}

// DuplicateLines copies the current line (or every line spanned by the
// selection) below itself, moving the cursor into the copy. Any selection
// collapses.
func (e *Editor) DuplicateLines() {
	sr, er := e.affectedLineRange()
	// The copy goes in as an insertion of n rows below er, an empty span there:
	// the undo step holds no lines at all, however large the document.
	e.pushUndoSpan(er+1, er+1)
	e.selecting, e.selBlock = false, false
	n := er - sr + 1
	e.doc.replaceRange(er+1, 0, cloneLines(e.doc.Lines()[sr:er+1]))
	// min: a cursor on the row a column-0 selection end leaves out
	// (affectedLineRange) lands on the copy's last line, not past it.
	e.cursorRow = min(e.cursorRow, er) + n
	e.clampCursor()
	e.ensureCursorVisible()
}

// DeleteLines removes the current line (or every line spanned by the
// selection). Any active selection collapses.
func (e *Editor) DeleteLines() {
	sr, er := e.affectedLineRange()
	e.pushUndoSpan(sr, er+1)
	e.selecting, e.selBlock = false, false
	var with [][]rune
	if er-sr+1 == e.doc.Len() {
		with = [][]rune{{}} // a document always keeps one line
	}
	e.doc.replaceRange(sr, er-sr+1, with)
	e.cursorRow, e.cursorCol = sr, 0
	e.clampCursor()
	e.ensureCursorVisible()
}

// MoveLinesUp swaps the current line (or every line spanned by the selection)
// with the line above. No-op at the top. Any selection is preserved and shifts
// with the moved lines.
func (e *Editor) MoveLinesUp() {
	sr, er := e.affectedLineRange()
	if sr == 0 {
		return
	}
	e.pushUndoSpan(sr-1, er+1)
	lines := e.doc.Lines()
	moved := append(slices.Clone(lines[sr:er+1]), lines[sr-1])
	e.doc.replaceRange(sr-1, len(moved), moved)
	e.cursorRow--
	if e.selecting {
		e.selAnchorRow--
	}
	e.clampCursor()
	e.ensureCursorVisible()
}

// MoveLinesDown mirrors MoveLinesUp, swapping downward. No-op at the
// bottom of the buffer.
func (e *Editor) MoveLinesDown() {
	sr, er := e.affectedLineRange()
	if er >= e.doc.Len()-1 {
		return
	}
	e.pushUndoSpan(sr, er+2)
	lines := e.doc.Lines()
	moved := append([][]rune{lines[er+1]}, lines[sr:er+1]...)
	e.doc.replaceRange(sr, len(moved), moved)
	e.cursorRow++
	if e.selecting {
		e.selAnchorRow++
	}
	// A selection end at column 0 of the row below the block (affectedLineRange
	// leaves that row out) now sits past the buffer when the block reached the
	// bottom: pin it to the end of the moved block's last line instead, so the
	// selection still covers exactly the moved lines and the anchor stays in range.
	last := e.doc.Len() - 1
	if e.cursorRow > last {
		e.cursorRow, e.cursorCol = last, len(e.doc.Line(last))
	}
	if e.selecting && e.selAnchorRow > last {
		e.selAnchorRow, e.selAnchorCol = last, len(e.doc.Line(last))
	}
	e.clampCursor()
	e.ensureCursorVisible()
}

// DefaultIndentWidth is how many spaces IndentLines and the Tab key insert in an
// Editor whose width has not been set. Tabs are never inserted, only converted
// away by DedentLines/dedentAmount, since Editor's rendering has no tab-stop
// expansion. A host that keeps its own default indent width (e.g. in its
// configuration) must keep it equal to this one.
const DefaultIndentWidth = 4

// defaultIndentWidth is what NewEditor seeds e.indentWidth with, so editors built
// where the application config is out of reach (property-sheet T-SQL rows, the
// Agent job-step command box) still honour the user's indent size; editors that
// can reach the config call SetIndentWidth instead.
var defaultIndentWidth = DefaultIndentWidth

// SetDefaultIndentWidth sets the width newly created Editors start with. Values
// outside 1..MaxIndentWidth are ignored. Existing editors keep theirs; use
// SetIndentWidth. Package-level state with no lock: call it on the UI
// goroutine, before the editors it should apply to are created.
func SetDefaultIndentWidth(n int) {
	if n < 1 || n > MaxIndentWidth {
		return
	}
	defaultIndentWidth = n
}

// MaxIndentWidth is the sanity ceiling SetIndentWidth and SetDefaultIndentWidth
// enforce; a host's own setting should clamp to the same range.
const MaxIndentWidth = 16

// SetIndentWidth sets how many spaces Tab, IndentLines, DedentLines and tab
// expansion use in this editor. Values outside 1..MaxIndentWidth are ignored.
//
// Call it at construction, before any SetText: SetText expands tabs at the current
// width, and a caller that snapshots Text() afterwards to detect edits would
// read a later width change as a user edit. Changing the width at
// runtime deliberately does not re-expand existing text.
func (e *Editor) SetIndentWidth(n int) {
	if n < 1 || n > MaxIndentWidth {
		return
	}
	e.indentWidth = n
}

// IndentWidth returns the editor's indent width in spaces.
func (e *Editor) IndentWidth() int { return e.indentWidth }

// expandTabs replaces every literal tab in text with e.indentWidth spaces so
// loaded or pasted content renders like typed indentation (a raw tab would draw
// as one narrow column).
func (e *Editor) expandTabs(text string) string {
	return strings.ReplaceAll(text, "\t", strings.Repeat(" ", e.indentWidth))
}

// SetIndentRule sets a language's smart-indent rule: Enter calls rule with the
// text left of the cursor, trailing blanks trimmed and never empty, and indents
// the new line by that many extra levels (of IndentWidth) beyond the line's own
// leading whitespace. nil, the default, adds none: a plain multi-line text box
// keeps only the copied indentation.
func (e *Editor) SetIndentRule(rule func(left string) int) { e.indentRule = rule }

// smartIndentBonus reports the extra columns Enter adds beyond the line's own
// leading whitespace, from the indent rule (see SetIndentRule).
func (e *Editor) smartIndentBonus() int {
	if e.indentRule == nil || e.cursorRow < 0 || e.cursorRow >= e.doc.Len() {
		return 0
	}
	line := e.doc.Line(e.cursorRow)
	left := strings.TrimRight(string(line[:min(e.cursorCol, len(line))]), " \t")
	if left == "" {
		return 0
	}
	return max(e.indentRule(left), 0) * e.indentWidth
}

// leadingIndentForNewLine reports how many leading whitespace runes the cursor's
// line starts with, clamped to the cursor column so splitting inside the
// indentation copies only what is left of the cursor (SSMS's smart indenting).
// Tabs can't occur in the buffer (expandTabs) but one counts as a rune anyway,
// mirroring dedentAmount.
func (e *Editor) leadingIndentForNewLine() int {
	if e.cursorRow < 0 || e.cursorRow >= e.doc.Len() {
		return 0
	}
	line := e.doc.Line(e.cursorRow)
	limit := min(e.cursorCol, len(line))
	n := 0
	for n < limit && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return n
}

// IndentLines inserts e.indentWidth spaces at column 0 of the current line (or
// every line spanned by the selection). An active selection is preserved, its
// columns shifted right on whichever row(s) the anchor/cursor sit.
func (e *Editor) IndentLines() {
	sr, er := e.affectedLineRange()
	e.pushUndoSpan(sr, er+1)
	for r := sr; r <= er; r++ {
		line := e.doc.Line(r)
		nl := make([]rune, len(line)+e.indentWidth)
		for i := range e.indentWidth {
			nl[i] = ' '
		}
		copy(nl[e.indentWidth:], line)
		e.doc.setLine(r, nl)
		if r == e.cursorRow {
			e.cursorCol += e.indentWidth
		}
		if e.selecting && r == e.selAnchorRow {
			e.selAnchorCol += e.indentWidth
		}
	}
	e.clampCursor()
	e.ensureCursorVisible()
}

// DedentLines removes one leading tab, or up to e.indentWidth leading spaces,
// from the current line (or every line spanned by the selection). An active
// selection is preserved, its columns shifted left by what was removed per row.
func (e *Editor) DedentLines() {
	sr, er := e.affectedLineRange()
	e.pushUndoSpan(sr, er+1)
	for r := sr; r <= er; r++ {
		line := e.doc.Line(r)
		removed := e.dedentAmount(line)
		if removed == 0 {
			continue
		}
		nl := make([]rune, len(line)-removed)
		copy(nl, line[removed:])
		e.doc.setLine(r, nl)
		if r == e.cursorRow {
			e.cursorCol = max(0, e.cursorCol-removed)
		}
		if e.selecting && r == e.selAnchorRow {
			e.selAnchorCol = max(0, e.selAnchorCol-removed)
		}
	}
	e.clampCursor()
	e.ensureCursorVisible()
}

// dedentAmount reports how many leading runes DedentLines should strip: one
// leading tab (content from before tabs were converted, or pasted in), else up to
// e.indentWidth leading spaces.
func (e *Editor) dedentAmount(line []rune) int {
	if len(line) > 0 && line[0] == '\t' {
		return 1
	}
	n := 0
	for n < len(line) && n < e.indentWidth && line[n] == ' ' {
		n++
	}
	return n
}

// SetLineComment sets the language's line-comment prefix ("--" for SQL, "//"
// for Go, "#" for a shell script) that ToggleLineComments (Ctrl+/) inserts and
// strips. Empty, the default, makes ToggleLineComments a no-op and leaves
// Ctrl+/ unclaimed: tuikit knows no language of its own.
func (e *Editor) SetLineComment(prefix string) { e.lineComment = []rune(prefix) }

// LineComment returns the prefix SetLineComment set ("" when none).
func (e *Editor) LineComment() string { return string(e.lineComment) }

// leadingSpace returns the index of line's first non-whitespace rune
// (len(line) for a blank line).
func leadingSpace(line []rune) int {
	i := 0
	for i < len(line) && unicode.IsSpace(line[i]) {
		i++
	}
	return i
}

// isCommentedLine reports whether line, after leading whitespace, starts with
// prefix.
func isCommentedLine(line, prefix []rune) bool {
	i := leadingSpace(line)
	return len(line)-i >= len(prefix) && slices.Equal(line[i:i+len(prefix)], prefix)
}

// commentLine inserts prefix and a space at line's first non-whitespace column
// (or appends them if the line is blank).
func commentLine(line, prefix []rune) []rune {
	i := leadingSpace(line)
	nl := make([]rune, 0, len(line)+len(prefix)+1)
	nl = append(nl, line[:i]...)
	nl = append(nl, prefix...)
	nl = append(nl, ' ')
	nl = append(nl, line[i:]...)
	return nl
}

// uncommentLine strips a leading prefix (and one following space) from line.
// No-op if line isn't commented.
func uncommentLine(line, prefix []rune) []rune {
	if !isCommentedLine(line, prefix) {
		return line
	}
	i := leadingSpace(line)
	j := i + len(prefix)
	if j < len(line) && line[j] == ' ' {
		j++
	}
	nl := make([]rune, 0, len(line)-(j-i))
	nl = append(nl, line[:i]...)
	nl = append(nl, line[j:]...)
	return nl
}

// ToggleLineComments comments or uncomments the current line (or every line
// spanned by the selection): uncomments only if every affected line is already
// commented, else comments every affected line (a blank line counts as "not
// commented", so it gets the prefix too). The prefix is SetLineComment's; with
// none set this does nothing. An active selection is preserved, its
// columns approximately shifted by the net length change on the anchor/cursor rows
// (a cursor inside leading whitespace can drift a column or two; accepted).
func (e *Editor) ToggleLineComments() {
	prefix := e.lineComment
	if len(prefix) == 0 {
		return
	}
	sr, er := e.affectedLineRange()
	allCommented := true
	for r := sr; r <= er; r++ {
		if !isCommentedLine(e.doc.Line(r), prefix) {
			allCommented = false
			break
		}
	}
	e.pushUndoSpan(sr, er+1)
	for r := sr; r <= er; r++ {
		before := len(e.doc.Line(r))
		if allCommented {
			e.doc.setLine(r, uncommentLine(e.doc.Line(r), prefix))
		} else {
			e.doc.setLine(r, commentLine(e.doc.Line(r), prefix))
		}
		delta := len(e.doc.Line(r)) - before
		if r == e.cursorRow {
			e.cursorCol = max(0, e.cursorCol+delta)
		}
		if e.selecting && r == e.selAnchorRow {
			e.selAnchorCol = max(0, e.selAnchorCol+delta)
		}
	}
	e.clampCursor()
	e.ensureCursorVisible()
}

// transformSelection applies fn to every rune in the selection, in place,
// branching on selBlock as SelectedText does. No-op with no selection. The
// rewritten lines go back through setLine even though the slice header is
// unchanged: the version counter tells the highlighters and wrap cache the text
// moved, and skipping it would leave an uppercased keyword in its old non-keyword
// colour until some unrelated edit bumped the version.
func (e *Editor) transformSelection(fn func(rune) rune) {
	if !e.HasSelection() {
		return
	}
	// selectionBounds' rows are the min/max of anchor and cursor in both modes.
	sr, _, er, _ := e.selectionBounds()
	e.pushUndoSpan(sr, er+1)
	apply := func(r, lo, hi int) {
		line := e.doc.Line(r)
		for i := lo; i < hi; i++ {
			line[i] = fn(line[i])
		}
		e.doc.setLine(r, line)
	}
	if e.selBlock {
		topRow, botRow := min(e.selAnchorRow, e.cursorRow), max(e.selAnchorRow, e.cursorRow)
		loCol, hiCol := e.blockColumnBounds()
		for r := topRow; r <= botRow; r++ {
			n := len(e.doc.Line(r))
			apply(r, core.Clamp(loCol, 0, n), core.Clamp(hiCol, 0, n))
		}
		return
	}
	_, sc, _, ec := e.selectionBounds()
	for r := sr; r <= er; r++ {
		n := len(e.doc.Line(r))
		lo, hi := 0, n
		if r == sr {
			lo = core.Clamp(sc, 0, n)
		}
		if r == er {
			hi = core.Clamp(ec, 0, n)
		}
		apply(r, lo, hi)
	}
}

// UppercaseSelection and LowercaseSelection convert the case of every rune in
// the selection, in place. No-op without a selection.
func (e *Editor) UppercaseSelection() { e.transformSelection(unicode.ToUpper) }
func (e *Editor) LowercaseSelection() { e.transformSelection(unicode.ToLower) }

// ReplaceText swaps the whole document for text as one undo step, which SetText
// can't be used for: it resets the history, so Ctrl+Z after Format Document would
// have nothing to restore. The cursor keeps its row and column, clamped into the
// new text; any selection collapses. Returns false, with no undo step, on a
// read-only editor or when text equals the current content.
func (e *Editor) ReplaceText(text string) bool {
	if e.readOnly {
		return false
	}
	if !e.replaceLines(0, e.doc.Len(), e.splitText(text)) {
		return false
	}
	e.selecting, e.selBlock = false, false
	e.clampCursor()
	e.ensureCursorVisible()
	return true
}

// SelectedLinesText returns the whole lines a linear selection touches (the
// affectedLineRange rows, joined with "\n"): what a line-oriented rewrite such as
// Format Document operates on. ok is false with no selection or a block (column)
// selection, which has no line span to rewrite.
func (e *Editor) SelectedLinesText() (text string, ok bool) {
	if !e.HasSelection() || e.selBlock {
		return "", false
	}
	sr, er := e.affectedLineRange()
	var sb strings.Builder
	for r := sr; r <= er; r++ {
		if r > sr {
			sb.WriteByte('\n')
		}
		sb.WriteString(string(e.doc.Line(r)))
	}
	return sb.String(), true
}

// ReplaceSelectedLines replaces the lines SelectedLinesText returns with text as
// one undo step, then selects the replacement's whole lines so the result is what
// a repeat of the command would act on. Returns false, changing nothing, where
// SelectedLinesText would report !ok, on a read-only editor, or when text equals
// those lines.
func (e *Editor) ReplaceSelectedLines(text string) bool {
	if e.readOnly || !e.HasSelection() || e.selBlock {
		return false
	}
	sr, er := e.affectedLineRange()
	lines := e.splitText(text)
	if !e.replaceLines(sr, er-sr+1, lines) {
		return false
	}
	e.selecting, e.selBlock = true, false
	e.selAnchorRow, e.selAnchorCol = sr, 0
	e.cursorRow = sr + len(lines) - 1
	e.cursorCol = len(e.doc.Line(e.cursorRow))
	e.ensureCursorVisible()
	return true
}

// replaceLines splices lines over the n rows at row as one undo step, through
// replaceRange so caches before row survive. Returns false, pushing no step, when
// lines equal what is already there: an unchanged rewrite must not mark the
// document modified or leave a no-op entry on the undo stack.
func (e *Editor) replaceLines(row, n int, lines [][]rune) bool {
	if slices.EqualFunc(e.doc.Lines()[row:row+n], lines, slices.Equal) {
		return false
	}
	e.closeCompletion()
	e.pushUndoSpan(row, row+n)
	e.doc.replaceRange(row, n, lines)
	return true
}

// deleteWordLeft removes the word to the left of the cursor (Ctrl+Backspace), or
// merges with the previous line at column 0. The caller (HandleKey) pushes undo.
func (e *Editor) deleteWordLeft() {
	if e.cursorCol == 0 {
		if e.cursorRow > 0 {
			e.backspace()
		}
		return
	}
	line := e.doc.Line(e.cursorRow)
	left := core.WordBoundaryLeft(line, e.cursorCol)
	e.doc.setLine(e.cursorRow, append(line[:left], line[e.cursorCol:]...))
	e.cursorCol = left
}

// deleteWordRight removes the word to the right of the cursor (Ctrl+Delete), or
// merges with the next line at end-of-line. The caller (HandleKey) pushes undo.
func (e *Editor) deleteWordRight() {
	line := e.doc.Line(e.cursorRow)
	if e.cursorCol >= len(line) {
		if e.cursorRow < e.doc.Len()-1 {
			e.deleteChar()
		}
		return
	}
	right := core.WordBoundaryRight(line, e.cursorCol)
	e.doc.setLine(e.cursorRow, append(line[:e.cursorCol], line[right:]...))
}
