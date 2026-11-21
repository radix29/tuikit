package controls

import (
	"github.com/gdamore/tcell/v3"

	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// TabBar is a one-row strip of plain tabs (no close buttons, no overflow) with
// an optional right-aligned text cell — a result-tab or page-tab row inside a
// panel or dialog. layout.PanelManager draws its own, richer tab row.
//
// The host owns which tabs exist and which is active: it sets Labels and
// Active before each Draw and HandleMouse (they are plain fields, read every
// call), and keyboard tab switching stays with it. Draw and hit-testing share
// one layout, core.TabStripSegments, so clicks land where tabs are drawn.
//
// Selecting a tab is idempotent — a press on the already-active tab fires
// nothing — so the resends tcell makes while Button1 is held cannot refire
// OnSelect, and TabBar holds no PressLatch. The Right cell is not idempotent
// (an Expand button, say), so TabBar never acts on it: the host hit-tests
// RightRect under its own gesture guard, ahead of HandleMouse.
type TabBar struct {
	// Labels are the tabs, each drawn padded " label ".
	Labels []string
	// Active is the index into Labels drawn highlighted; out of range
	// highlights none.
	Active int
	// Inset is the number of bar columns left blank before the first tab.
	Inset int
	// BarStyle fills the bar and draws inactive tabs and the Right cell; the
	// zero Style means theme.StyleMenuBar().
	BarStyle tcell.Style
	// Right, when non-empty, is drawn right-aligned with a one-column margin,
	// and only if it fits clear of the last tab (see RightRect).
	Right string
	// OnSelect fires when a press lands on a tab other than Active, after
	// Active is set to it.
	OnSelect func(i int)

	rect core.Rect
}

// SetBounds places the bar at (x, y), w columns wide, one row tall.
func (t *TabBar) SetBounds(x, y, w int) {
	t.rect = core.Rect{X: x, Y: y, W: max(0, w), H: 1}
}

// Bounds returns the bar's rect.
func (t *TabBar) Bounds() core.Rect { return t.rect }

// Segments returns each drawn tab's extent (one TabSegment per tab, its padded
// label), in Labels order; tabs that don't fit are left off the end.
func (t *TabBar) Segments() [][]core.TabSegment {
	widths := make([][]int, len(t.Labels))
	for i, label := range t.Labels {
		widths[i] = []int{core.TabLabelWidth(label)}
	}
	return core.TabStripSegments(t.rect.X+t.Inset, widths, t.rect.Right())
}

// TabAt returns the index of the tab drawn at (x, y), or -1.
func (t *TabBar) TabAt(x, y int) int {
	if t.rect.W <= 0 || y != t.rect.Y {
		return -1
	}
	for i, seg := range t.Segments() {
		if x >= seg[0].X && x < seg[0].X+seg[0].W {
			return i
		}
	}
	return -1
}

// RightRect returns where the Right cell is drawn, or the zero Rect when Right
// is empty or doesn't fit: it needs a blank column after the last tab and one
// before the bar's right edge.
func (t *TabBar) RightRect() core.Rect {
	if t.Right == "" || t.rect.W <= 0 {
		return core.Rect{}
	}
	col := t.rect.X + t.Inset
	if segs := t.Segments(); len(segs) > 0 {
		last := segs[len(segs)-1][0]
		col = last.X + last.W + 1
	}
	w := core.DisplayWidth(t.Right)
	x := t.rect.Right() - w - 1
	if x < col {
		return core.Rect{}
	}
	return core.Rect{X: x, Y: t.rect.Y, W: w, H: 1}
}

func (t *TabBar) barStyle() tcell.Style {
	if t.BarStyle == (tcell.Style{}) {
		return theme.StyleMenuBar()
	}
	return t.BarStyle
}

// activeTabStyle is the highlighted tab, the same as an active panel title.
func activeTabStyle() tcell.Style {
	return theme.StyleTitleActive()
}

// Draw fills the bar, then draws the tabs and the Right cell.
func (t *TabBar) Draw(s tcell.Screen) {
	if t.rect.W <= 0 {
		return
	}
	bar := t.barStyle()
	core.FillRect(s, t.rect, ' ', bar)
	for i, seg := range t.Segments() {
		st := bar
		if i == t.Active {
			st = activeTabStyle()
		}
		core.DrawText(s, seg[0].X, t.rect.Y, st, " "+t.Labels[i]+" ")
	}
	if r := t.RightRect(); r.W > 0 {
		core.DrawText(s, r.X, r.Y, bar, t.Right)
	}
}

// HandleMouse selects the tab under a Button1 press (see the type comment on
// resends) and reports whether the event was a Button1 on the bar — a press
// between tabs, or on the Right cell, is consumed without action.
func (t *TabBar) HandleMouse(ev *tcell.EventMouse) bool {
	if ev.Buttons() != tcell.Button1 {
		return false
	}
	x, y := ev.Position()
	if !t.rect.Contains(x, y) {
		return false
	}
	if i := t.TabAt(x, y); i >= 0 && i != t.Active {
		t.Active = i
		if t.OnSelect != nil {
			t.OnSelect(i)
		}
	}
	return true
}
