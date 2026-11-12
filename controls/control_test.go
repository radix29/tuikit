package controls

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/radix29/tuikit/controls/editor"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

// Which of core's control interfaces each control implements; see the widgets
// package's control_test.go for why these are pinned.
var (
	_ interface {
		core.Focusable
		core.Bounded
		core.KeyHandler
		core.MouseHandler
	} = (*ListBox)(nil)
	_ interface {
		core.Focusable
		core.Bounded
		core.KeyHandler
		core.MouseHandler
		core.Overlay
	} = (*TreeView)(nil)
	_ interface {
		core.Focusable
		core.Bounded
		core.KeyHandler
		core.MouseHandler
		core.Overlay
		core.ClipboardTarget
	} = (*DataGrid)(nil)
	_ interface {
		core.Focusable
		core.Bounded
		core.KeyHandler
		core.MouseHandler
		core.Overlay
		core.ClipboardTarget
	} = (*editor.Editor)(nil)
	_ interface {
		core.Bounded
		core.KeyHandler
		core.MouseHandler
		core.Overlay
	} = (*MenuBar)(nil)
	_ interface {
		core.KeyHandler
		core.MouseHandler
		core.Overlay
	} = (*ContextMenu)(nil)
	_ interface {
		core.Bounded
		core.MouseHandler
		core.Overlay
	} = (*Toolbar)(nil)
	_ interface {
		core.KeyHandler
		core.MouseHandler
		core.Overlay
		core.ClipboardTarget
	} = (*ValueViewer)(nil)
)

func TestControlsReportFocus(t *testing.T) {
	for _, c := range []core.Focusable{NewListBox(), NewTreeView(), NewDataGrid(), editor.NewEditor(nil)} {
		c.Focus(true)
		if !c.Focused() {
			t.Errorf("%T: Focused() = false after Focus(true)", c)
		}
		c.Focus(false)
		if c.Focused() {
			t.Errorf("%T: Focused() = true after Focus(false)", c)
		}
	}
}

func TestControlsReportTheBoundsTheyWereGiven(t *testing.T) {
	want := core.Rect{X: 2, Y: 3, W: 20, H: 7}
	for _, c := range []interface {
		core.Bounded
		SetBounds(x, y, w, h int)
	}{NewListBox(), NewTreeView(), NewDataGrid(), editor.NewEditor(nil)} {
		c.SetBounds(want.X, want.Y, want.W, want.H)
		if got := c.Bounds(); got != want {
			t.Errorf("%T: Bounds() = %+v, want %+v", c, got, want)
		}
	}
}

// The contract tests below (AR2) run every control in this package through
// the rules of core.MouseHandler and core.KeyHandler and tuikit's
// ARCHITECTURE.md § Mouse gestures. Most bugs of the 2026-11-12 review were
// one of these rules broken in one more control; a new control joins each
// table.

// tallLines is enough content for every scrolling control to show a
// scrollbar in the 40x10 rect the contract tests give it.
func tallLines() []string {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d with enough text to scroll sideways too", i)
	}
	return lines
}

func contractListBox() *ListBox {
	l := NewListBox()
	l.SetBounds(0, 0, 40, 10)
	l.SetItems(tallLines())
	return l
}

func contractTreeView() *TreeView {
	tv := NewTreeView()
	tv.SetBounds(0, 0, 40, 10)
	var nodes []TreeNode
	for i, s := range tallLines() {
		nodes = append(nodes, TreeNode{ID: i + 1, Label: s})
	}
	tv.SetNodes(nodes)
	return tv
}

func contractDataGrid() *DataGrid {
	g := NewDataGrid()
	g.SetBounds(0, 0, 40, 10)
	var rows [][]string
	for _, s := range tallLines() {
		rows = append(rows, []string{s, s, s})
	}
	g.SetData([]string{"First", "Second", "Third"}, rows)
	g.SetCellCursor(true)
	return g
}

func contractEditor(wrap bool) *editor.Editor {
	e := editor.NewEditor(nil)
	e.SetBounds(0, 0, 40, 10)
	e.SetText(strings.Join(tallLines(), "\n"))
	e.SetWrapMode(wrap)
	return e
}

func contractToolbar() *Toolbar {
	tb := NewToolbar()
	tb.SetButtons([]ToolbarButton{{Icon: "Run"}, {Icon: "Stop"}})
	tb.SetBounds(0, 0, 40)
	return tb
}

// Plain motion is never claimed, at any point of a control: a host reads a
// claimed hover as a press and moves focus (BG1: the property sheet's page
// list stole focus from the field being typed in).
func TestContractHoverIsNotClaimed(t *testing.T) {
	controls := []struct {
		name string
		c    core.MouseHandler
	}{
		{"ListBox", contractListBox()},
		{"ListBox empty", NewListBox()},
		{"TreeView", contractTreeView()},
		{"DataGrid", contractDataGrid()},
		{"DataGrid row mode", func() *DataGrid { g := contractDataGrid(); g.SetCellCursor(false); return g }()},
		{"Editor", contractEditor(false)},
		{"Editor wrapped", contractEditor(true)},
		{"Toolbar", contractToolbar()},
		{"MenuBar", newTestMenuBar()},
	}
	for _, tc := range controls {
		// Every cell of the 40x10 rect: content, gutter, header, row header,
		// scrollbars, status row, a toolbar's buttons and a menu bar's labels.
		for y := range 10 {
			for x := range 40 {
				if tc.c.HandleMouse(tuikittest.Mouse(x, y, tcell.ButtonNone)) {
					t.Errorf("%s: hover at (%d,%d) claimed", tc.name, x, y)
					break
				}
			}
		}
	}
}

// A release anywhere — inside, or far outside the control after a drag left
// it — ends every gesture the control was holding: afterwards neither plain
// motion nor a fresh press outside is claimed. A latch that survived its
// release would turn the next motion into a drag of whatever it held.
func TestContractReleaseAnywhereClearsEveryLatch(t *testing.T) {
	type control interface {
		core.MouseHandler
		core.Overlay
	}
	controls := []struct {
		name    string
		mk      func() control
		presses [][2]int // inside points, one per zone that can start a gesture
	}{
		{"ListBox", func() control { return listBoxOverlay{contractListBox()} }, [][2]int{{3, 3}, {39, 5}}},
		{"TreeView", func() control { return contractTreeView() }, [][2]int{{3, 3}, {39, 5}, {10, 9}}},
		// Cell, row header, column header, header separator, vertical bar,
		// horizontal bar.
		{"DataGrid", func() control { return contractDataGrid() }, [][2]int{{12, 4}, {1, 4}, {12, 1}, {0, 1}, {39, 5}, {20, 9}}},
		{"DataGrid row mode", func() control { g := contractDataGrid(); g.SetCellCursor(false); return g }, [][2]int{{12, 4}, {12, 1}, {39, 5}}},
		{"Editor", func() control { return contractEditor(false) }, [][2]int{{12, 4}, {1, 4}, {39, 5}, {20, 9}}},
		{"Editor wrapped", func() control { return contractEditor(true) }, [][2]int{{12, 4}, {39, 5}}},
		{"Toolbar", func() control { return contractToolbar() }, [][2]int{{1, 0}, {8, 0}}},
	}
	for _, tc := range controls {
		for _, p := range tc.presses {
			for _, rel := range [][2]int{p, {70, 30}} {
				c := tc.mk()
				c.HandleMouse(tuikittest.Press(p[0], p[1]))
				c.HandleMouse(tuikittest.Mouse(p[0]+1, p[1]+1, tcell.Button1)) // a drag step
				c.HandleMouse(tuikittest.Release(rel[0], rel[1]))
				if c.OverlayActive() {
					continue // a press that opened a popup: the overlay rules apply
				}
				if c.HandleMouse(tuikittest.Mouse(5, 5, tcell.ButtonNone)) {
					t.Errorf("%s: press at %v, release at %v: next motion claimed", tc.name, p, rel)
				}
				if c.HandleMouse(tuikittest.Press(70, 30)) {
					t.Errorf("%s: press at %v, release at %v: a later press outside claimed", tc.name, p, rel)
				}
			}
		}
	}
}

// listBoxOverlay gives ListBox, which never floats anything, the Overlay half
// of the release test's interface.
type listBoxOverlay struct{ *ListBox }

func (listBoxOverlay) DrawOverlay(tcell.Screen) {}
func (listBoxOverlay) OverlayActive() bool      { return false }

// Navigation keys on empty content neither panic nor leave state the next
// Draw indexes out of range (BG4: PgDn on an empty TreeView set its scroll to
// -1). Each control is tried sized and before its first SetBounds.
func TestContractNavKeysOnEmptyContentKeepDrawWorking(t *testing.T) {
	keys := []tcell.Key{
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn,
		tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight, tcell.KeyEnter,
	}
	type control interface {
		core.KeyHandler
		Draw(tcell.Screen)
	}
	mks := []struct {
		name string
		mk   func() control
	}{
		{"ListBox", func() control { return NewListBox() }},
		{"ListBox emptied", func() control { l := contractListBox(); l.SetItems(nil); return l }},
		{"TreeView", func() control { return NewTreeView() }},
		{"TreeView emptied", func() control { tv := contractTreeView(); tv.SetNodes(nil); return tv }},
		{"DataGrid", func() control { return NewDataGrid() }},
		{"DataGrid no rows", func() control { g := NewDataGrid(); g.SetData([]string{"A"}, nil); return g }},
		{"DataGrid no rows cell cursor", func() control {
			g := NewDataGrid()
			g.SetData([]string{"A"}, nil)
			g.SetCellCursor(true)
			return g
		}},
		{"Editor", func() control { return editor.NewEditor(nil) }},
		{"Editor wrapped", func() control { e := editor.NewEditor(nil); e.SetWrapMode(true); return e }},
		{"MenuBar", func() control { return NewMenuBar() }},
	}
	for _, tc := range mks {
		for _, sized := range []bool{true, false} {
			for _, mod := range []tcell.ModMask{tcell.ModNone, tcell.ModShift, tcell.ModCtrl} {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s (sized %v, mod %v): panic: %v", tc.name, sized, mod, r)
						}
					}()
					c := tc.mk()
					if sized {
						switch b := c.(type) {
						case interface{ SetBounds(x, y, w, h int) }:
							b.SetBounds(0, 0, 40, 10)
						case interface{ SetBounds(x, y, w int) }:
							b.SetBounds(0, 0, 40)
						}
					}
					if f, ok := c.(core.Focusable); ok {
						f.Focus(true)
					}
					scr := tuikittest.NewScreen(80, 24)
					for _, k := range keys {
						c.HandleKey(tuikittest.Key(k, mod))
						c.Draw(scr)
						if o, ok := c.(core.Overlay); ok {
							o.DrawOverlay(scr)
						}
					}
				}()
			}
		}
	}
}
