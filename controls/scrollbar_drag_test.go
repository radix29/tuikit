package controls

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

// TestDataGridScrollbarDragScrolls confirms a Button1 press on the
// scrollbar column (rect.Right()-1) jumps scrollRow proportionally instead
// of being read as a row click, and that a subsequent drag (still Button1,
// x drifted off the bar's exact column) keeps controlling the scroll.
func TestDataGridScrollbarDragScrolls(t *testing.T) {
	g := newTestDataGrid() // 40x10 -> dataH = 7
	cols := []string{"A"}
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"x"}
	}
	g.SetData(cols, rows)
	sbX := g.rect.Right() - 1

	if !g.HandleMouse(tcell.NewEventMouse(sbX, g.rect.Y+2, tcell.Button1, tcell.ModNone)) {
		t.Fatal("HandleMouse on scrollbar column should be handled")
	}
	if g.scrollRow != 0 {
		t.Errorf("scrollRow after clicking track top = %d, want 0", g.scrollRow)
	}
	if !g.gestureIs(gridVBar) {
		t.Fatal("pressing on the scrollbar should start a thumb drag")
	}
	if g.selRow != 0 {
		t.Errorf("selRow = %d, clicking the scrollbar must not also select a row", g.selRow)
	}

	// Drag downward, x drifted left off the bar's exact column — the drag
	// must still control scroll, not fall through to row selection.
	g.HandleMouse(tcell.NewEventMouse(sbX-2, g.rect.Y+2+6, tcell.Button1, tcell.ModNone))
	if g.scrollRow == 0 {
		t.Error("scrollRow should have advanced after dragging toward the bottom of the track")
	}
	if g.selRow != 0 {
		t.Errorf("selRow = %d, a scrollbar drag must never change row selection", g.selRow)
	}

	g.HandleMouse(tcell.NewEventMouse(sbX-2, g.rect.Y+2+6, tcell.ButtonNone, tcell.ModNone))
	if g.gestureZone() != nil {
		t.Error("the thumb drag should end on release")
	}
}

// TestTreeViewScrollbarDragScrolls mirrors the DataGrid case for TreeView,
// whose scrollbar is drawn over the border column rather than real content.
func TestTreeViewScrollbarDragScrolls(t *testing.T) {
	tv := NewTreeView()
	tv.SetBounds(0, 0, 40, 10) // inner.H = 8
	nodes := make([]TreeNode, 20)
	for i := range nodes {
		nodes[i] = TreeNode{ID: TreeNodeID(i + 1), Label: "n"}
	}
	tv.SetNodes(nodes)
	sbX := tv.rect.Right() - 1
	inner := tv.rect.Inner(1)

	if !tv.HandleMouse(tcell.NewEventMouse(sbX, inner.Y+inner.H-1, tcell.Button1, tcell.ModNone)) {
		t.Fatal("HandleMouse on scrollbar column should be handled")
	}
	if z, held := tv.gesture.Owner(); !held || z != treeVBar {
		t.Fatal("pressing on the scrollbar should start a thumb drag")
	}
	if tv.scroll == 0 {
		t.Error("scroll should have jumped forward when clicking near the bottom of the track")
	}
	if tv.sel != 0 {
		t.Errorf("sel = %d, clicking the scrollbar must not also select a node", tv.sel)
	}

	tv.HandleMouse(tcell.NewEventMouse(sbX, inner.Y, tcell.ButtonNone, tcell.ModNone))
	if _, held := tv.gesture.Owner(); held {
		t.Error("the thumb drag should end on release")
	}
}

// TestListBoxScrollbarDragScrolls mirrors the DataGrid case for ListBox.
func TestListBoxScrollbarDragScrolls(t *testing.T) {
	items := make([]string, 20)
	for i := range items {
		items[i] = "item"
	}
	l := newTestListBox(items...)
	sbX := l.rect.Right() - 1

	if !l.HandleMouse(tcell.NewEventMouse(sbX, l.rect.Y+l.rect.H-1, tcell.Button1, tcell.ModNone)) {
		t.Fatal("HandleMouse on scrollbar column should be handled")
	}
	if !l.sbDragging {
		t.Fatal("sbDragging should be true after pressing on the scrollbar")
	}
	if l.scroll == 0 {
		t.Error("scroll should have jumped forward when clicking near the bottom of the track")
	}
	if l.sel != 0 {
		t.Errorf("sel = %d, clicking the scrollbar must not also select an item", l.sel)
	}

	l.HandleMouse(tcell.NewEventMouse(sbX, l.rect.Y, tcell.ButtonNone, tcell.ModNone))
	if l.sbDragging {
		t.Error("sbDragging should reset on release")
	}
}
