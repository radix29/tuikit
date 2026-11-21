package controls

import (
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
	"github.com/radix29/tuikit/tuikittest"
)

func TestTabBarDrawsTabsAndHighlightsActive(t *testing.T) {
	s := tuikittest.NewScreen(30, 1)
	b := &TabBar{Labels: []string{"Plan", "Tree", "XML"}, Active: 1, Inset: 1}
	b.SetBounds(0, 0, 30)
	b.Draw(s)
	if got, want := s.Row(0), "  Plan   Tree   XML           "; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
	x, _, _ := s.Find("Tree")
	if got := s.StyleAt(x, 0); got != activeTabStyle() {
		t.Errorf("active tab style = %v, want activeTabStyle", got)
	}
	x, _, _ = s.Find("Plan")
	if got := s.StyleAt(x, 0); got != theme.StyleMenuBar() {
		t.Errorf("inactive tab style = %v, want StyleMenuBar (zero BarStyle)", got)
	}
}

func TestTabBarBarStyleOverridesDefault(t *testing.T) {
	s := tuikittest.NewScreen(20, 1)
	bar := theme.StyleDialog()
	b := &TabBar{Labels: []string{"A"}, Active: -1, BarStyle: bar}
	b.SetBounds(0, 0, 20)
	b.Draw(s)
	if got := s.StyleAt(19, 0); got != bar {
		t.Errorf("bar fill style = %v, want BarStyle", got)
	}
}

// Draw and hit-test share Segments: every drawn label's columns map back to
// its own index, and the gaps and the inset to -1.
func TestTabBarTabAtMatchesDrawnLabels(t *testing.T) {
	s := tuikittest.NewScreen(40, 2)
	labels := []string{"Results 1", "Results 2", "Messages"}
	b := &TabBar{Labels: labels, Inset: 1}
	b.SetBounds(0, 1, 40)
	b.Draw(s)
	for i, l := range labels {
		x, y, ok := s.Find(l)
		if !ok || y != 1 {
			t.Fatalf("label %q not drawn on row 1", l)
		}
		for c := x - 1; c < x+core.DisplayWidth(l)+1; c++ {
			if got := b.TabAt(c, 1); got != i {
				t.Errorf("TabAt(%d) = %d, want %d (%q)", c, got, i, l)
			}
		}
	}
	if got := b.TabAt(0, 1); got != -1 {
		t.Errorf("TabAt(inset) = %d, want -1", got)
	}
	segs := b.Segments()
	gap := segs[0][0].X + segs[0][0].W
	if got := b.TabAt(gap, 1); got != -1 {
		t.Errorf("TabAt(gap) = %d, want -1", got)
	}
	if got := b.TabAt(segs[0][0].X, 0); got != -1 {
		t.Errorf("TabAt(other row) = %d, want -1", got)
	}
}

func TestTabBarDropsTabsThatDoNotFit(t *testing.T) {
	b := &TabBar{Labels: []string{"One", "Two", "Three"}}
	b.SetBounds(0, 0, 11) // " One " + gap + " Two " = 11
	if got := len(b.Segments()); got != 2 {
		t.Fatalf("segments = %d, want 2", got)
	}
	s := tuikittest.NewScreen(11, 1)
	b.Draw(s)
	if got, want := s.Row(0), " One   Two "; got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
}

func TestTabBarHandleMouseSelectsOnceAndConsumesBar(t *testing.T) {
	var fired []int
	b := &TabBar{Labels: []string{"Login", "Connection String"}, OnSelect: func(i int) { fired = append(fired, i) }}
	b.SetBounds(5, 2, 40)
	x := b.Segments()[1][0].X + 1

	if !b.HandleMouse(tuikittest.Press(x, 2)) {
		t.Fatal("press on a tab not consumed")
	}
	// tcell resends the held button on every motion: no refire.
	b.HandleMouse(tuikittest.Press(x+1, 2))
	if len(fired) != 1 || fired[0] != 1 || b.Active != 1 {
		t.Fatalf("fired %v, Active %d; want [1], 1", fired, b.Active)
	}
	if !b.HandleMouse(tuikittest.Press(44, 2)) {
		t.Error("press on the bar past the tabs not consumed")
	}
	if b.HandleMouse(tuikittest.Press(x, 3)) {
		t.Error("press below the bar consumed")
	}
	if b.HandleMouse(tuikittest.Release(x, 2)) || b.HandleMouse(tuikittest.Mouse(x, 2, tcell.Button2)) {
		t.Error("non-Button1 event consumed")
	}
	if len(fired) != 1 {
		t.Errorf("fired %v after non-tab events, want only [1]", fired)
	}
}

func TestTabBarRightCellPlacementAndFit(t *testing.T) {
	b := &TabBar{Labels: []string{"Plan", "Tree"}, Inset: 1, Right: "[ Expand ]"}
	b.SetBounds(0, 0, 40)
	r := b.RightRect()
	if want := (core.Rect{X: 40 - 10 - 1, Y: 0, W: 10, H: 1}); r != want {
		t.Fatalf("RightRect = %+v, want %+v", r, want)
	}
	s := tuikittest.NewScreen(40, 1)
	b.Draw(s)
	if x, _, ok := s.Find("[ Expand ]"); !ok || x != r.X {
		t.Errorf("Right drawn at %d (found %v), want %d", x, ok, r.X)
	}

	// Too narrow to keep a blank column after the last tab: not drawn, no rect.
	b.SetBounds(0, 0, 25)
	if r := b.RightRect(); r != (core.Rect{}) {
		t.Errorf("narrow RightRect = %+v, want zero", r)
	}
	s = tuikittest.NewScreen(25, 1)
	b.Draw(s)
	if _, _, ok := s.Find("Expand"); ok {
		t.Error("Right drawn though it does not fit")
	}

	b.Right = ""
	b.SetBounds(0, 0, 40)
	if r := b.RightRect(); r != (core.Rect{}) {
		t.Errorf("empty Right: RightRect = %+v, want zero", r)
	}
}

func TestTabBarZeroWidthDrawsAndHitsNothing(t *testing.T) {
	s := tuikittest.NewScreen(10, 1)
	b := &TabBar{Labels: []string{"A"}}
	b.SetBounds(0, 0, 0)
	b.Draw(s)
	if got := s.Row(0); got != "          " {
		t.Errorf("row = %q, want blank", got)
	}
	if b.TabAt(0, 0) != -1 || b.HandleMouse(tuikittest.Press(0, 0)) {
		t.Error("zero-width bar hit")
	}
}

func TestTabBarRightCellBoundaryFit(t *testing.T) {
	b := &TabBar{Labels: []string{"ab", "cd"}, Inset: 1, Right: "xy"}
	b.SetBounds(0, 0, 40)
	segs := b.Segments()
	last := segs[len(segs)-1][0]
	col := last.X + last.W + 1 // first column after the one blank column
	w := col + 2 + 1           // Right at col, one blank column before the edge

	b.SetBounds(0, 0, w)
	if r, want := b.RightRect(), (core.Rect{X: col, Y: 0, W: 2, H: 1}); r != want {
		t.Errorf("boundary width %d: RightRect = %+v, want %+v", w, r, want)
	}
	b.SetBounds(0, 0, w-1)
	if r := b.RightRect(); r != (core.Rect{}) {
		t.Errorf("one column narrower: RightRect = %+v, want zero", r)
	}

	// No tabs: nothing to keep a blank column from, so the cell may start at the inset.
	n := &TabBar{Inset: 1, Right: "xy"}
	n.SetBounds(0, 0, 4)
	if r, want := n.RightRect(), (core.Rect{X: 1, Y: 0, W: 2, H: 1}); r != want {
		t.Errorf("no tabs: RightRect = %+v, want %+v", r, want)
	}
}
