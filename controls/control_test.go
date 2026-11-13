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

// A key that changes nothing is not claimed (core.KeyHandler): a host falls
// back to its own navigation only on false, so a control answering true at an
// edge is a keyboard trap (IN1: propsheet.Form could not leave a grid or a
// script box by Up/Down). Each control is tried at both ends of its content
// and empty; the same keys away from the end must be claimed, so the test
// can't pass by a control that claims nothing.
func TestContractKeyThatChangesNothingIsNotClaimed(t *testing.T) {
	up := []tcell.Key{tcell.KeyUp, tcell.KeyPgUp, tcell.KeyHome}
	down := []tcell.Key{tcell.KeyDown, tcell.KeyPgDn, tcell.KeyEnd}
	all := append(append(append([]tcell.Key{}, up...), down...), tcell.KeyLeft, tcell.KeyRight, tcell.KeyEnter)
	type edge struct {
		name   string
		mk     func() core.KeyHandler
		toEnd  []tcell.Key // keys that move a fresh control to the far end
		atHead []tcell.Key // unclaimed on a fresh control
		atTail []tcell.Key // unclaimed after toEnd
	}
	gridEnd := []tcell.Key{tcell.KeyEnd, tcell.KeyRight, tcell.KeyRight}
	edges := []edge{
		// Enter: no OnActivate. Left/Right are not bound.
		{"ListBox", func() core.KeyHandler { return contractListBox() }, []tcell.Key{tcell.KeyEnd},
			append(append([]tcell.Key{}, up...), tcell.KeyEnter, tcell.KeyLeft), down},
		// Left: nothing expanded. Enter and Right: no children, no OnActivate.
		{"TreeView", func() core.KeyHandler { return contractTreeView() }, []tcell.Key{tcell.KeyEnd},
			append(append([]tcell.Key{}, up...), tcell.KeyLeft, tcell.KeyRight, tcell.KeyEnter), down},
		// Enter: a read-only grid activates nothing.
		{"DataGrid", func() core.KeyHandler { return contractDataGrid() }, gridEnd,
			append(append([]tcell.Key{}, up...), tcell.KeyLeft, tcell.KeyEnter), append(append([]tcell.Key{}, down...), tcell.KeyRight)},
		{"DataGrid row mode", func() core.KeyHandler { g := contractDataGrid(); g.SetCellCursor(false); return g }, []tcell.Key{tcell.KeyEnd},
			append(append([]tcell.Key{}, up...), tcell.KeyLeft, tcell.KeyEnter), down},
		{"Editor", func() core.KeyHandler { return contractEditor(false) }, nil,
			append(append([]tcell.Key{}, up...), tcell.KeyLeft), nil},
		{"Editor wrapped", func() core.KeyHandler { return contractEditor(true) }, nil,
			append(append([]tcell.Key{}, up...), tcell.KeyLeft), nil},
		{"ListBox empty", func() core.KeyHandler { return NewListBox() }, nil, all, nil},
		{"TreeView empty", func() core.KeyHandler { tv := NewTreeView(); tv.SetBounds(0, 0, 40, 10); return tv }, nil, all, nil},
		{"DataGrid empty", func() core.KeyHandler {
			g := NewDataGrid()
			g.SetBounds(0, 0, 40, 10)
			g.SetData([]string{"A"}, nil)
			g.SetCellCursor(true)
			return g
		}, nil, all, nil},
	}
	editorEnd := func(e *editor.Editor) { e.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModCtrl)) }
	for _, tc := range edges {
		for _, mod := range []tcell.ModMask{tcell.ModNone, tcell.ModShift} {
			for _, k := range tc.atHead {
				if c := tc.mk(); c.HandleKey(tuikittest.Key(k, mod)) {
					t.Errorf("%s: %v (mod %v) at the start claimed", tc.name, k, mod)
				}
			}
			tail := tc.atTail
			if _, isEditor := tc.mk().(*editor.Editor); isEditor {
				tail = append(append([]tcell.Key{}, down...), tcell.KeyRight)
			}
			for _, k := range tail {
				c := tc.mk()
				if e, ok := c.(*editor.Editor); ok {
					editorEnd(e)
				}
				for _, m := range tc.toEnd {
					c.HandleKey(tuikittest.Key(m, tcell.ModNone))
				}
				if c.HandleKey(tuikittest.Key(k, mod)) {
					t.Errorf("%s: %v (mod %v) at the end claimed", tc.name, k, mod)
				}
			}
		}
		// The same control moves on Down from its start, unless it is empty.
		if len(tc.atTail) > 0 || tc.toEnd == nil && !strings.HasSuffix(tc.name, "empty") {
			if !tc.mk().HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModNone)) {
				t.Errorf("%s: Down from the start not claimed", tc.name)
			}
		}
	}
}

// A drag acts only through the zone its press began in (ARCHITECTURE.md § One
// owner per press inside a widget): from a scrollbar it scrolls and never
// selects, from a press that already acted (a header, a tree row, a list
// item, a toolbar button) it does nothing at all, wherever it wanders. BG2: a
// cell drag reaching a separator resized the column, one reaching the header
// re-sorted.
func TestContractDragActsOnlyThroughItsOwnZone(t *testing.T) {
	type state = string
	type control struct {
		name  string
		mk    func() core.MouseHandler
		press [2]int
		// sel reads what the drag must not change; for a press that acted, scroll
		// is included.
		sel func(core.MouseHandler) state
	}
	listSel := func(c core.MouseHandler) state { l := c.(*ListBox); return fmt.Sprint(l.sel) }
	listAll := func(c core.MouseHandler) state { l := c.(*ListBox); return fmt.Sprint(l.sel, l.scroll) }
	treeSel := func(c core.MouseHandler) state { tv := c.(*TreeView); return fmt.Sprint(tv.sel) }
	treeAll := func(c core.MouseHandler) state {
		tv := c.(*TreeView)
		return fmt.Sprint(tv.sel, tv.scroll, tv.scrollX)
	}
	gridSel := func(c core.MouseHandler) state {
		g := c.(*DataGrid)
		r0, c0, r1, c1 := g.SelectionBounds()
		return fmt.Sprint(g.selRow, g.selCol, r0, c0, r1, c1, g.marking)
	}
	gridAll := func(c core.MouseHandler) state {
		g := c.(*DataGrid)
		return fmt.Sprint(gridSel(c), g.scrollRow, g.scrollCol, g.colWidths)
	}
	edSel := func(c core.MouseHandler) state {
		e := c.(*editor.Editor)
		r, col := e.CursorPos()
		return fmt.Sprint(r, col, e.HasSelection(), e.SelectedText())
	}
	var clicks int
	toolbar := func() core.MouseHandler {
		tb := NewToolbar()
		tb.SetButtons([]ToolbarButton{{Icon: "Run", Action: func() { clicks++ }}, {Icon: "Stop", Action: func() { clicks++ }}})
		tb.SetBounds(0, 0, 40)
		return tb
	}
	controls := []control{
		{"ListBox item", func() core.MouseHandler { return contractListBox() }, [2]int{3, 3}, listAll},
		{"ListBox bar", func() core.MouseHandler { return contractListBox() }, [2]int{39, 5}, listSel},
		{"TreeView row", func() core.MouseHandler { return contractTreeView() }, [2]int{3, 3}, treeAll},
		{"TreeView bar", func() core.MouseHandler { return contractTreeView() }, [2]int{39, 5}, treeSel},
		{"TreeView hbar", func() core.MouseHandler { return contractTreeView() }, [2]int{10, 9}, treeSel},
		{"DataGrid header", func() core.MouseHandler { return contractDataGrid() }, [2]int{12, 0}, gridAll},
		{"DataGrid bar", func() core.MouseHandler { return contractDataGrid() }, [2]int{39, 5}, gridSel},
		{"DataGrid hbar", func() core.MouseHandler { return contractDataGrid() }, [2]int{20, 9}, gridSel},
		{"DataGrid separator", func() core.MouseHandler { return contractDataGrid() }, [2]int{0, 1}, gridSel},
		{"Editor bar", func() core.MouseHandler { return contractEditor(false) }, [2]int{39, 5}, edSel},
		{"Editor hbar", func() core.MouseHandler { return contractEditor(false) }, [2]int{20, 9}, edSel},
		{"Editor wrapped bar", func() core.MouseHandler { return contractEditor(true) }, [2]int{39, 5}, edSel},
		// The buttons are right-aligned, and the rect is theirs alone: 30 is Run.
		{"Toolbar button", toolbar, [2]int{30, 0}, func(core.MouseHandler) state { return fmt.Sprint(clicks) }},
	}
	for _, tc := range controls {
		c := tc.mk()
		c.HandleMouse(tuikittest.Press(tc.press[0], tc.press[1]))
		want := tc.sel(c)
		// Every cell of the rect, then past each edge.
		path := [][2]int{{-5, 5}, {60, 5}, {20, -3}, {20, 20}}
		for y := range 10 {
			for x := range 40 {
				path = append(path, [2]int{x, y})
			}
		}
		for _, p := range path {
			c.HandleMouse(tuikittest.Mouse(p[0], p[1], tcell.Button1))
			if got := tc.sel(c); got != want {
				t.Errorf("%s: drag from %v reached %v and changed %q to %q", tc.name, tc.press, p, want, got)
				break
			}
		}
		c.HandleMouse(tuikittest.Release(5, 5))
	}
}

// SetBounds with the same rect changes nothing; a new rect never moves the
// selection or cursor and leaves the scroll inside the content (IN3). Each
// control starts scrolled to its far end, where a taller or wider rect has the
// most room to leave blank rows above the content: BG5 for TreeView, and a
// wrapped Editor's scrollRow, counting visual rows, pointed past the end once
// a wider rect made them fewer.
func TestContractSetBoundsKeepsSelectionAndScrollInContent(t *testing.T) {
	type control struct {
		name string
		mk   func() interface {
			SetBounds(x, y, w, h int)
			Draw(tcell.Screen)
		}
		state  func(c any) string // selection or cursor, then scroll
		inside func(c any) bool   // scroll within the content, drawn or not
	}
	editorTop := func(c any) bool {
		e := c.(*editor.Editor)
		r := e.Bounds()
		s := tuikittest.NewScreen(r.W, r.H)
		e.Draw(s)
		return strings.TrimSpace(s.Row(0)) != "" // a line number or text, not past the end
	}
	editorState := func(c any) string {
		e := c.(*editor.Editor)
		row, col := e.CursorPos()
		sr, sc := e.ScrollPos()
		return fmt.Sprint(row, col, sr, sc)
	}
	toEditorEnd := func(e *editor.Editor) *editor.Editor {
		e.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModCtrl))
		return e
	}
	controls := []control{
		{"ListBox", func() interface {
			SetBounds(x, y, w, h int)
			Draw(tcell.Screen)
		} {
			l := contractListBox()
			l.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModNone))
			return l
		}, func(c any) string { l := c.(*ListBox); return fmt.Sprint(l.sel, l.scroll) },
			func(c any) bool { l := c.(*ListBox); return l.scroll >= 0 && l.scroll <= max(0, len(l.items)-l.rect.H) }},
		{"TreeView", func() interface {
			SetBounds(x, y, w, h int)
			Draw(tcell.Screen)
		} {
			tv := contractTreeView()
			tv.HandleKey(tuikittest.Key(tcell.KeyEnd, tcell.ModNone))
			tv.HandleMouse(tuikittest.Mouse(5, 5, tcell.WheelRight, tcell.ModNone))
			return tv
		}, func(c any) string { tv := c.(*TreeView); return fmt.Sprint(tv.sel, tv.scroll, tv.scrollX) },
			func(c any) bool {
				tv := c.(*TreeView)
				in := tv.rect.Inner(1)
				return tv.scroll >= 0 && tv.scroll <= max(0, len(tv.nodes)-in.H) &&
					tv.scrollX >= 0 && tv.scrollX <= max(0, tv.contentW-in.W)
			}},
		{"DataGrid", func() interface {
			SetBounds(x, y, w, h int)
			Draw(tcell.Screen)
		} {
			g := contractDataGrid()
			for _, k := range []tcell.Key{tcell.KeyEnd, tcell.KeyRight, tcell.KeyRight} {
				g.HandleKey(tuikittest.Key(k, tcell.ModNone))
			}
			return g
		}, func(c any) string {
			g := c.(*DataGrid)
			return fmt.Sprint(g.selRow, g.selCol, g.scrollRow, g.scrollCol)
		},
			func(c any) bool {
				g := c.(*DataGrid)
				return g.scrollRow >= 0 && g.scrollRow <= max(0, g.rows.Len()-g.dataRows())
			}},
		{"Editor", func() interface {
			SetBounds(x, y, w, h int)
			Draw(tcell.Screen)
		} {
			return toEditorEnd(contractEditor(false))
		}, editorState, editorTop},
		{"Editor wrapped", func() interface {
			SetBounds(x, y, w, h int)
			Draw(tcell.Screen)
		} {
			return toEditorEnd(contractEditor(true))
		}, editorState, editorTop},
	}
	for _, tc := range controls {
		c := tc.mk()
		before := tuikittest.NewScreen(40, 10)
		c.Draw(before)
		st := tc.state(c)
		c.SetBounds(0, 0, 40, 10)
		after := tuikittest.NewScreen(40, 10)
		c.Draw(after)
		if got := tc.state(c); got != st || after.Snapshot() != before.Snapshot() {
			t.Errorf("%s: SetBounds with the same rect changed state %q -> %q or the drawing", tc.name, st, got)
		}
		for _, r := range []core.Rect{{W: 40, H: 60}, {W: 120, H: 10}, {W: 120, H: 60}, {W: 20, H: 5}, {W: 25, H: 10}} {
			c := tc.mk()
			sel := strings.Fields(tc.state(c))
			c.SetBounds(r.X, r.Y, r.W, r.H)
			got := strings.Fields(tc.state(c))
			// Selection or cursor is every field but the scroll ones; the
			// controls above put them first: one for ListBox and TreeView,
			// two for DataGrid and Editor.
			n := 2
			if tc.name == "ListBox" || tc.name == "TreeView" {
				n = 1
			}
			if strings.Join(got[:n], " ") != strings.Join(sel[:n], " ") {
				t.Errorf("%s: SetBounds(%+v) moved the selection %v -> %v", tc.name, r, sel[:n], got[:n])
			}
			if !tc.inside(c) {
				t.Errorf("%s: SetBounds(%+v) left the scroll outside the content (%v)", tc.name, r, got)
			}
		}
	}
}

// One wheel tick scrolls every control by the same step (IN4): core.WheelRows
// down, core.WheelCells sideways where the control scrolls by the cell. Form
// stepped three rows and the others one.
func TestContractWheelStep(t *testing.T) {
	l := contractListBox()
	tv := contractTreeView()
	g := contractDataGrid()
	e := contractEditor(false)
	for _, c := range []core.MouseHandler{l, tv, g, e} {
		c.HandleMouse(tuikittest.Mouse(5, 5, tcell.WheelDown, tcell.ModNone))
	}
	er, _ := e.ScrollPos()
	for name, got := range map[string]int{"ListBox": l.scroll, "TreeView": tv.scroll, "DataGrid": g.scrollRow, "Editor": er} {
		if got != core.WheelRows {
			t.Errorf("%s: WheelDown scrolled %d rows, want %d", name, got, core.WheelRows)
		}
	}
	tv.HandleMouse(tuikittest.Mouse(5, 5, tcell.WheelRight, tcell.ModNone))
	e.HandleMouse(tuikittest.Mouse(5, 5, tcell.WheelRight, tcell.ModNone))
	_, ec := e.ScrollPos()
	if tv.scrollX != core.WheelCells || ec != core.WheelCells {
		t.Errorf("WheelRight: TreeView %d, Editor %d cells, want %d", tv.scrollX, ec, core.WheelCells)
	}
}
