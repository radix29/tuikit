package editor

import (
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// The editor stress test drives one Editor through a seeded random sequence of
// keys, mouse events, resizes and public actions, checking after every step
// that Draw and DrawOverlay survive and the cursor (and any selection) is
// inside the document, and at the end that undoing everything restores the
// text it started with. It found the stale selection Undo()/Redo() left behind
// (a panic on the next key, reachable from gossms's Edit menu); unit tests
// written per feature never combine an API action with a key-made selection.
//
// A failure reports the seed and the last operations; runEditorMonkey(t, seed)
// replays that one seed.

const (
	editorMonkeySeeds = 300
	editorMonkeyOps   = 300
)

// monkeyText is the starting document: plain, wide (CJK), combining, emoji,
// tabs, an empty line and trailing spaces, so cursor and column math meet
// every kind of rune.
const monkeyText = "select a, b\n\tfrom t -- 注释\n\nwhere x = 'é' and y = '👍🏽'\n  é end  "

var monkeyRunes = []rune{'a', 'Z', ' ', '\t', '-', '/', '\'', '注', '漢', 'e', '́', '👍', 'é'}

var monkeyKeys = []tcell.Key{
	tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
	tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn,
	tcell.KeyEnter, tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete,
	tcell.KeyTab, tcell.KeyBacktab, tcell.KeyEscape, tcell.KeyInsert,
	tcell.KeyCtrlZ, tcell.KeyCtrlY,
}

var monkeyMods = []tcell.ModMask{
	tcell.ModNone, tcell.ModNone, tcell.ModNone,
	tcell.ModShift, tcell.ModShift,
	tcell.ModCtrl, tcell.ModAlt,
	tcell.ModCtrl | tcell.ModShift, tcell.ModAlt | tcell.ModShift,
}

var monkeyButtons = []tcell.ButtonMask{
	tcell.Button1, tcell.Button1, tcell.ButtonNone, tcell.Button2,
	tcell.WheelUp, tcell.WheelDown, tcell.WheelLeft, tcell.WheelRight,
}

// monkeySearches never put a line break in Replace: replaceMatch inserts a
// multi-line replacement as runes into one row, leaving a raw '\n' inside a
// document line — open for a decision (split it into rows, or refuse it in
// SetSearch), and not what this test is after.
var monkeySearches = []SearchOptions{
	{Query: "a", Replace: "AA"},
	{Query: "e", Replace: "", MatchCase: true},
	{Query: "where", Replace: "WHERE", WholeWord: true},
	{Query: `\s+`, Replace: " ", Regexp: true},
	{Query: "注", Replace: "👍"},
	{Query: "x", Replace: "yy", InSelection: true},
}

func randomMonkeyText(r *rand.Rand) string {
	var b strings.Builder
	for range r.IntN(12) {
		if r.IntN(5) == 0 {
			b.WriteByte('\n')
		} else {
			b.WriteRune(monkeyRunes[r.IntN(len(monkeyRunes))])
		}
	}
	return b.String()
}

type editorMonkey struct {
	r   *rand.Rand
	e   *Editor
	s   *tuikittest.Screen
	ops []string
}

// step performs one random operation and returns its description.
func (m *editorMonkey) step() string {
	r, e := m.r, m.e
	b := e.Bounds()
	switch n := r.IntN(100); {
	case n < 35:
		k := monkeyKeys[r.IntN(len(monkeyKeys))]
		mod := monkeyMods[r.IntN(len(monkeyMods))]
		ev := tuikittest.Key(k, mod)
		e.HandleKey(ev)
		return "key " + ev.Name()
	case n < 55:
		c := monkeyRunes[r.IntN(len(monkeyRunes))]
		mod := tcell.ModNone
		if r.IntN(6) == 0 {
			mod = monkeyMods[r.IntN(len(monkeyMods))]
		}
		ev := tuikittest.Rune(c, mod)
		e.HandleKey(ev)
		return fmt.Sprintf("rune %q (%s)", c, ev.Name())
	case n < 70:
		// Mostly inside the rect, sometimes just outside it (a drag leaving
		// the editor, a press on a neighbour).
		x := b.X - 2 + r.IntN(max(1, b.W+4))
		y := b.Y - 2 + r.IntN(max(1, b.H+4))
		btn := monkeyButtons[r.IntN(len(monkeyButtons))]
		mod := tcell.ModNone
		if r.IntN(4) == 0 {
			mod = monkeyMods[r.IntN(len(monkeyMods))]
		}
		e.HandleMouse(tuikittest.Mouse(x, y, btn, mod))
		return fmt.Sprintf("mouse %d,%d btn %v mod %v", x, y, btn, mod)
	case n < 74:
		w, h := r.IntN(30), r.IntN(10)
		e.SetBounds(r.IntN(3), r.IntN(3), w, h)
		return fmt.Sprintf("SetBounds %dx%d", w, h)
	case n < 76:
		v := !e.WrapMode()
		e.SetWrapMode(v)
		return fmt.Sprintf("SetWrapMode(%v)", v)
	case n < 80:
		t := randomMonkeyText(r)
		e.Paste(t)
		return fmt.Sprintf("Paste(%q)", t)
	case n < 82:
		e.Cut()
		return "Cut"
	case n < 85:
		opts := monkeySearches[r.IntN(len(monkeySearches))]
		if err := e.SetSearch(opts); err != nil {
			return fmt.Sprintf("SetSearch(%+v) err %v", opts, err)
		}
		switch r.IntN(4) {
		case 0:
			e.FindNext(1)
			return fmt.Sprintf("SetSearch(%+v) FindNext(1)", opts)
		case 1:
			e.FindNext(-1)
			return fmt.Sprintf("SetSearch(%+v) FindNext(-1)", opts)
		case 2:
			e.ReplaceCurrent()
			return fmt.Sprintf("SetSearch(%+v) ReplaceCurrent", opts)
		default:
			e.ReplaceAll()
			return fmt.Sprintf("SetSearch(%+v) ReplaceAll", opts)
		}
	default:
		return m.action()
	}
}

// action is one of the editor's public commands — what a host's menus call,
// bypassing HandleKey's own selection bookkeeping.
func (m *editorMonkey) action() string {
	r, e := m.r, m.e
	switch r.IntN(18) {
	case 0:
		e.Undo()
		return "Undo"
	case 1:
		e.Redo()
		return "Redo"
	case 2:
		e.DuplicateLines()
		return "DuplicateLines"
	case 3:
		e.DeleteLines()
		return "DeleteLines"
	case 4:
		e.MoveLinesUp()
		return "MoveLinesUp"
	case 5:
		e.MoveLinesDown()
		return "MoveLinesDown"
	case 6:
		e.IndentLines()
		return "IndentLines"
	case 7:
		e.DedentLines()
		return "DedentLines"
	case 8:
		e.ToggleLineComments()
		return "ToggleLineComments"
	case 9:
		e.UppercaseSelection()
		return "UppercaseSelection"
	case 10:
		e.LowercaseSelection()
		return "LowercaseSelection"
	case 11:
		t := randomMonkeyText(r)
		e.ReplaceText(t)
		return fmt.Sprintf("ReplaceText(%q)", t)
	case 12:
		t := randomMonkeyText(r)
		e.ReplaceSelectedLines(t)
		return fmt.Sprintf("ReplaceSelectedLines(%q)", t)
	case 13:
		row := r.IntN(e.doc.Len()+4) - 2
		e.GoToLine(row)
		return fmt.Sprintf("GoToLine(%d)", row)
	case 14:
		e.SelectAll()
		return "SelectAll"
	case 15:
		n := e.doc.Len()
		ar, rw := r.IntN(n+2)-1, r.IntN(n+2)-1
		ac, c := r.IntN(12)-1, r.IntN(12)-1
		e.SelectRange(ar, ac, rw, c)
		return fmt.Sprintf("SelectRange(%d,%d,%d,%d)", ar, ac, rw, c)
	case 16:
		e.ClearSearch()
		return "ClearSearch"
	default:
		e.ClearSelection()
		return "ClearSelection"
	}
}

// check is the per-step invariant: everything that reads the cursor or the
// selection survives, and both lie inside the document.
func (m *editorMonkey) check() error {
	e := m.e
	e.Draw(m.s)
	e.DrawOverlay(m.s)
	_ = e.SelectedText()
	_, _ = e.SelectedLinesText()
	_ = e.WordAtCursor()
	if e.HasSelection() {
		_ = e.SelectionTopRow()
	}
	n := e.doc.Len()
	if n < 1 {
		return fmt.Errorf("document has %d lines", n)
	}
	if e.cursorRow < 0 || e.cursorRow >= n {
		return fmt.Errorf("cursor row %d outside [0,%d)", e.cursorRow, n)
	}
	if l := len(e.doc.Line(e.cursorRow)); e.cursorCol < 0 || e.cursorCol > l {
		return fmt.Errorf("cursor col %d outside [0,%d] on row %d", e.cursorCol, l, e.cursorRow)
	}
	if e.selecting {
		if e.selAnchorRow < 0 || e.selAnchorRow >= n {
			return fmt.Errorf("selection anchor row %d outside [0,%d)", e.selAnchorRow, n)
		}
		// A block selection is rune-indexed per row and may sit past a short
		// row's end (a ragged edge); only a linear anchor must be in its row.
		if l := len(e.doc.Line(e.selAnchorRow)); !e.selBlock && (e.selAnchorCol < 0 || e.selAnchorCol > l) {
			return fmt.Errorf("selection anchor col %d outside [0,%d] on row %d", e.selAnchorCol, l, e.selAnchorRow)
		}
	}
	return nil
}

func (m *editorMonkey) fail(t *testing.T, seed uint64, why any) {
	t.Helper()
	tail := m.ops[max(0, len(m.ops)-25):]
	t.Fatalf("seed %d, op %d: %v\nlast operations:\n  %s", seed, len(m.ops), why, strings.Join(tail, "\n  "))
}

func TestEditorSurvivesRandomOperations(t *testing.T) {
	seeds := uint64(editorMonkeySeeds)
	if testing.Short() {
		seeds /= 10
	}
	for seed := range seeds {
		runEditorMonkey(t, seed)
	}
}

func runEditorMonkey(t *testing.T, seed uint64) {
	t.Helper()
	m := &editorMonkey{
		r: rand.New(rand.NewPCG(seed, 0x6d6f6e6b6579)),
		e: NewEditor(nil),
		s: tuikittest.NewScreen(40, 14),
	}
	m.e.SetText(monkeyText)
	m.e.SetLineComment("--")
	m.e.SetBounds(0, 0, 30, 10)
	m.e.Focus(true)
	initial := m.e.Text()

	var failure any
	func() {
		defer func() {
			if p := recover(); p != nil {
				failure = fmt.Sprintf("panic: %v\n%s", p, debug.Stack())
			}
		}()
		for range editorMonkeyOps {
			m.ops = append(m.ops, m.step())
			if err := m.check(); err != nil {
				failure = err
				return
			}
		}
		// Undo everything. A step count above the operation count reaches the
		// bottom of the stack (300 operations stay under maxUndoSteps).
		for range 2 * editorMonkeyOps {
			m.e.Undo()
		}
		m.ops = append(m.ops, "Undo × all")
		if err := m.check(); err != nil {
			failure = err
		} else if got := m.e.Text(); got != initial {
			failure = fmt.Errorf("undoing everything left %q, want %q", got, initial)
		}
	}()
	if failure != nil {
		m.fail(t, seed, failure)
	}
}
