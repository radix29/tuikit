package controls

import (
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/radix29/tuikit/controls/editor"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

// core.MouseHandler's rule (IN2): a wheel tick over a control is claimed even
// where it cannot scroll, so it never scrolls whatever lies beneath. Hover is
// TestContractHoverIsNotClaimed's.
func TestControlsAlwaysClaimTheWheel(t *testing.T) {
	ed := editor.NewEditor(nil)
	ed.SetBounds(0, 0, 40, 10)
	ed.SetText("a\nb")
	wrapped := editor.NewEditor(nil)
	wrapped.SetBounds(0, 0, 40, 10)
	wrapped.SetText("a\nb")
	wrapped.SetWrapMode(true)

	controls := []struct {
		name string
		c    core.MouseHandler
	}{
		{"ListBox", newTestListBox("a", "b")},
		{"TreeView", newTestTreeView()},
		{"DataGrid", newCellCursorGrid()},
		{"Editor", ed},
		{"Editor wrapped", wrapped},
	}
	for _, tc := range controls {
		// At the top already; (1,3) is an editor's gutter, (10,3) a tree's blank
		// rows below its one node.
		for _, p := range [][2]int{{1, 3}, {10, 3}} {
			for _, b := range []tcell.ButtonMask{tcell.WheelUp, tcell.WheelDown} {
				if !tc.c.HandleMouse(tuikittest.Mouse(p[0], p[1], b)) {
					t.Errorf("%s: wheel %v at %v fell through", tc.name, b, p)
				}
			}
		}
	}
}

// A release that ends a gesture the control held is still claimed, so a host
// knows the drag ended there.
func TestDataGridClaimsOnlyTheReleaseOfItsOwnGesture(t *testing.T) {
	g := newCellCursorGrid()
	g.HandleMouse(tuikittest.Press(1, 2))
	if !g.HandleMouse(tuikittest.Release(1, 2)) {
		t.Error("release ending a cell press not claimed")
	}
	if g.HandleMouse(tuikittest.Release(1, 2)) {
		t.Error("plain motion after the release claimed")
	}
}

// Any press outside an open menu dismisses it. A right-click falls through so
// the host can open the next menu where it landed; a left click is consumed,
// and a wheel tick is consumed with the menu left open.
func TestMenusDismissOnAnyPressOutside(t *testing.T) {
	newCM := func() *ContextMenu {
		cm := &ContextMenu{}
		cm.Show(0, 0, []MenuItem{{Label: "Copy"}})
		return cm
	}
	newMB := func() *MenuBar {
		mb := newTestMenuBar()
		mb.Open()
		return mb
	}
	type menu interface {
		core.MouseHandler
		OverlayActive() bool
	}
	cases := []struct {
		btn          tcell.ButtonMask
		consumed     bool
		stillVisible bool
	}{
		{tcell.Button1, true, false},
		{tcell.Button2, false, false},
		{tcell.Button3, true, false},
		{tcell.WheelDown, true, true},
	}
	for name, mk := range map[string]func() menu{
		"ContextMenu": func() menu { return newCM() },
		"MenuBar":     func() menu { return newMB() },
	} {
		for _, tc := range cases {
			m := mk()
			// Far below either menu, and off the menu bar's row.
			got := m.HandleMouse(tuikittest.Mouse(35, 20, tc.btn))
			if got != tc.consumed {
				t.Errorf("%s: %v outside consumed = %v, want %v", name, tc.btn, got, tc.consumed)
			}
			if m.OverlayActive() != tc.stillVisible {
				t.Errorf("%s: %v outside left the menu open = %v, want %v", name, tc.btn, m.OverlayActive(), tc.stillVisible)
			}
		}
	}
}

// A right-click elsewhere in a grid whose context menu is open opens the menu
// again at the new cell, rather than only closing the old one.
func TestDataGridRightClickOutsideItsMenuReopensIt(t *testing.T) {
	g := NewDataGrid()
	g.SetBounds(0, 0, 70, 10)
	g.SetData([]string{"A column wide enough to click", "And a second one past the menu"},
		[][]string{{"1", "2"}, {"3", "4"}})
	g.SetCellCursor(true)
	g.HandleMouse(tuikittest.Mouse(1, 2, tcell.Button2))
	if !g.OverlayActive() {
		t.Fatal("right-click on a cell opened no menu")
	}
	g.HandleMouse(tuikittest.Release(1, 2))
	g.HandleMouse(tuikittest.Mouse(1, 30, tcell.Button2)) // outside the menu and the grid
	if g.OverlayActive() {
		t.Fatal("right-click off the grid left its menu open")
	}
	g.HandleMouse(tuikittest.Mouse(1, 2, tcell.Button2))
	if !g.OverlayActive() {
		t.Fatal("menu not shown after closing")
	}
	g.HandleMouse(tuikittest.Release(1, 2))
	g.DrawOverlay(tuikittest.NewScreen(80, 24)) // places the menu, so its rect is known
	x, y := g.ctxMenu.x, g.ctxMenu.y
	// A cell of the second column on the last row, past the menu's right edge.
	menu := g.ctxMenu.drawnRect()
	cx := menu.Right()
	for ; cx < g.rect.Right(); cx++ {
		if col, ok := g.colAt(cx); ok && col == 1 {
			break
		}
	}
	if cx == g.rect.Right() {
		t.Fatal("no second-column cell right of the menu")
	}
	g.HandleMouse(tuikittest.Mouse(cx, 3, tcell.Button2))
	if !g.OverlayActive() || g.ctxMenu.x == x && g.ctxMenu.y == y {
		t.Errorf("right-click on another cell: menu open = %v at (%d,%d), want reopened away from (%d,%d)",
			g.OverlayActive(), g.ctxMenu.x, g.ctxMenu.y, x, y)
	}
}
