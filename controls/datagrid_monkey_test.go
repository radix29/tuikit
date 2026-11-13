package controls

import (
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// The grid stress test drives one DataGrid through a seeded random sequence of
// keys, mouse events, data reloads, resizes (down to rects too short for a
// data row) and mode toggles, checking after every step that Draw and
// DrawOverlay survive and the selection names a real row and column. It found
// PgUp/PgDn paging by a negative height on a grid under three rows high (a
// panic in Draw once the grid grew again, reachable in gossms by shrinking the
// terminal); the per-feature tests each held the height fixed.
//
// A failure reports the seed and the last operations; runGridMonkey(t, seed)
// replays that one seed.

const (
	gridMonkeySeeds = 150
	gridMonkeyOps   = 300
)

var gridMonkeyKeys = []tcell.Key{
	tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
	tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn,
	tcell.KeyEnter, tcell.KeyEscape, tcell.KeyTab, tcell.KeyBacktab,
}

var gridMonkeyMods = []tcell.ModMask{
	tcell.ModNone, tcell.ModNone, tcell.ModNone,
	tcell.ModShift, tcell.ModCtrl, tcell.ModAlt,
	tcell.ModCtrl | tcell.ModShift, tcell.ModAlt | tcell.ModShift,
}

var gridMonkeyButtons = []tcell.ButtonMask{
	tcell.Button1, tcell.Button1, tcell.ButtonNone, tcell.Button2,
	tcell.WheelUp, tcell.WheelDown, tcell.WheelLeft, tcell.WheelRight,
}

var gridMonkeyCells = []string{"", "1", "abc", "注释漢字", "👍🏽", "é", "a\tb", "line1\nline2", strings.Repeat("w", 40)}

// randomGridData is 0–3 columns and 0–40 rows; every row as wide as the
// header, as SetData requires.
func randomGridData(r *rand.Rand) ([]string, [][]string) {
	cols := make([]string, r.IntN(4))
	for i := range cols {
		cols[i] = "col" + strconv.Itoa(i)
	}
	rows := make([][]string, r.IntN(41))
	for i := range rows {
		rows[i] = make([]string, len(cols))
		for j := range rows[i] {
			rows[i][j] = gridMonkeyCells[r.IntN(len(gridMonkeyCells))]
		}
	}
	return cols, rows
}

type gridMonkey struct {
	r    *rand.Rand
	g    *DataGrid
	s    *tuikittest.Screen
	cols int
	rows int
	ops  []string
}

func (m *gridMonkey) step() string {
	r, g := m.r, m.g
	b := g.Bounds()
	switch n := r.IntN(100); {
	case n < 40:
		k := gridMonkeyKeys[r.IntN(len(gridMonkeyKeys))]
		ev := tuikittest.Key(k, gridMonkeyMods[r.IntN(len(gridMonkeyMods))])
		g.HandleKey(ev)
		return "key " + ev.Name()
	case n < 45:
		c := []rune{' ', 'a', 'c', 'x'}[r.IntN(4)]
		ev := tuikittest.Rune(c, gridMonkeyMods[r.IntN(len(gridMonkeyMods))])
		g.HandleKey(ev)
		return "rune " + ev.Name()
	case n < 70:
		x := b.X - 2 + r.IntN(max(1, b.W+4))
		y := b.Y - 2 + r.IntN(max(1, b.H+4))
		btn := gridMonkeyButtons[r.IntN(len(gridMonkeyButtons))]
		mod := tcell.ModNone
		if r.IntN(4) == 0 {
			mod = gridMonkeyMods[r.IntN(len(gridMonkeyMods))]
		}
		g.HandleMouse(tuikittest.Mouse(x, y, btn, mod))
		return fmt.Sprintf("mouse %d,%d btn %v mod %v", x, y, btn, mod)
	case n < 80:
		// Heights 0–2 are the short-grid case B2 was; weight them.
		h := r.IntN(14)
		if r.IntN(3) == 0 {
			h = r.IntN(3)
		}
		w := r.IntN(40)
		g.SetBounds(r.IntN(3), r.IntN(3), w, h)
		return fmt.Sprintf("SetBounds %dx%d", w, h)
	case n < 85:
		cols, rows := randomGridData(r)
		m.cols, m.rows = len(cols), len(rows)
		g.SetData(cols, rows)
		return fmt.Sprintf("SetData %d cols × %d rows", m.cols, m.rows)
	case n < 90:
		cols, rows := randomGridData(r)
		m.cols, m.rows = len(cols), len(rows)
		g.SetDataPreservingView(cols, rows)
		return fmt.Sprintf("SetDataPreservingView %d cols × %d rows", m.cols, m.rows)
	case n < 92:
		v := !g.CellCursorEnabled()
		g.SetCellCursor(v)
		return fmt.Sprintf("SetCellCursor(%v)", v)
	case n < 94:
		v := !g.BrowseOnly()
		g.SetBrowseOnly(v)
		return fmt.Sprintf("SetBrowseOnly(%v)", v)
	case n < 96:
		g.ShowValue()
		return "ShowValue"
	case n < 97:
		g.SelectAll()
		return "SelectAll"
	case n < 98:
		row, col := r.IntN(m.rows+4)-2, r.IntN(m.cols+4)-2
		g.SetSelectedCell(row, col)
		return fmt.Sprintf("SetSelectedCell(%d,%d)", row, col)
	case n < 99:
		row, col := r.IntN(m.rows+4)-2, r.IntN(m.cols+4)-2
		g.SetScroll(row, col)
		return fmt.Sprintf("SetScroll(%d,%d)", row, col)
	default:
		g.ClearMarkedRows()
		return "ClearMarkedRows"
	}
}

// check is the per-step invariant: everything that reads the selection
// survives, and the selected row and cell exist whenever there are rows.
func (m *gridMonkey) check() error {
	g := m.g
	g.Draw(m.s)
	g.DrawOverlay(m.s)
	_ = g.SelectedText()
	_ = g.SelectedCellsText()
	_ = g.Status()
	if m.rows == 0 {
		return nil
	}
	if r := g.SelectedRow(); r < 0 || r >= m.rows {
		return fmt.Errorf("SelectedRow() = %d, want 0..%d", r, m.rows-1)
	}
	if m.cols > 0 && g.CellCursorEnabled() {
		if r, c := g.SelectedCell(); r < 0 || r >= m.rows || c < 0 || c >= m.cols {
			return fmt.Errorf("SelectedCell() = %d,%d, want within %d×%d", r, c, m.rows, m.cols)
		}
	}
	for _, r := range g.SelectedRows() {
		if r < 0 || r >= m.rows {
			return fmt.Errorf("SelectedRows() holds %d, want 0..%d", r, m.rows-1)
		}
		_ = g.Row(r)
	}
	if s := g.ScrollRow(); s < 0 || s >= m.rows {
		return fmt.Errorf("ScrollRow() = %d, want 0..%d", s, m.rows-1)
	}
	return nil
}

func TestDataGridSurvivesRandomOperations(t *testing.T) {
	seeds := uint64(gridMonkeySeeds)
	if testing.Short() {
		seeds /= 10
	}
	for seed := range seeds {
		runGridMonkey(t, seed)
	}
}

func runGridMonkey(t *testing.T, seed uint64) {
	t.Helper()
	m := &gridMonkey{
		r: rand.New(rand.NewPCG(seed, 0x67726964)),
		g: NewDataGrid(),
		s: tuikittest.NewScreen(45, 18),
	}
	cols, rows := randomGridData(m.r)
	m.cols, m.rows = len(cols), len(rows)
	m.g.SetData(cols, rows)
	m.g.SetBounds(0, 0, 30, 10)
	m.g.Focus(true)

	var failure any
	func() {
		defer func() {
			if p := recover(); p != nil {
				failure = fmt.Sprintf("panic: %v\n%s", p, debug.Stack())
			}
		}()
		for range gridMonkeyOps {
			m.ops = append(m.ops, m.step())
			if err := m.check(); err != nil {
				failure = err
				return
			}
		}
	}()
	if failure != nil {
		tail := m.ops[max(0, len(m.ops)-25):]
		t.Fatalf("seed %d, op %d: %v\nlast operations:\n  %s", seed, len(m.ops), failure, strings.Join(tail, "\n  "))
	}
}
