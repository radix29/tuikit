package controls

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// tallMenu builds a single menu of n enabled items, the shape that outgrows a
// short terminal: Edit ships 21, which needs 23 rows for its box.
func tallMenu(n int, ran *string) []Menu {
	items := make([]MenuItem, n)
	for i := range items {
		label := fmt.Sprintf("Item %d", i)
		items[i] = MenuItem{Label: label, Action: func() { *ran = label }}
	}
	return []Menu{{Label: "Edit", Items: items}}
}

// openTallMenuBar returns a MenuBar with one n-item dropdown open, already
// drawn once against an h-row screen so the geometry cache is populated.
func openTallMenuBar(n, h int, ran *string) (*MenuBar, *tuikittest.Screen) {
	mb := NewMenuBar()
	mb.SetBounds(0, 0, 80)
	mb.SetMenus(tallMenu(n, ran))
	mb.Open()
	s := tuikittest.NewScreen(80, h)
	mb.DrawOverlay(s)
	return mb, s
}

func TestDropdownTallerThanTheScreenIsClampedToIt(t *testing.T) {
	// 21 items want a 23-row box hanging off row 1; a 20-row screen has 19.
	mb, _ := openTallMenuBar(21, 20, new(string))
	r := mb.dropdownRect()
	if r.Y+r.H > 20 {
		t.Fatalf("dropdown box is rows %d..%d on a 20-row screen; want it to end by row 20 "+
			"(an unclamped box paints its last items past the bottom edge, "+
			"where they are invisible and unclickable)", r.Y, r.Y+r.H-1)
	}
	if r.H < 3 {
		t.Fatalf("dropdown height = %d, want at least 3 (two borders and a row)", r.H)
	}
}

func TestDropdownNarrowerScreenNeverGivesANegativeColumn(t *testing.T) {
	mb := NewMenuBar()
	mb.SetBounds(0, 0, 10)
	mb.SetMenus([]Menu{{Label: "Edit", Items: []MenuItem{
		{Label: "An item with a very long label indeed", Shortcut: "Ctrl+Shift+X"},
	}}})
	mb.Open()
	mb.DrawOverlay(tuikittest.NewScreen(10, 24))
	r := mb.dropdownRect()
	if r.X < 0 || r.W < 1 {
		t.Fatalf("dropdown box = %+v on a 10-column screen; want X >= 0 and W >= 1 "+
			"(shifting a too-wide box left off the right edge pushes it off the left one)", r)
	}
}

func TestKeyboardScrollsAnOverTallDropdownToTheSelectedItem(t *testing.T) {
	var ran string
	mb, s := openTallMenuBar(21, 20, &ran)
	rows := mb.dropdownRect().H - 2

	// Walk to the last item, redrawing between keys the way the event loop
	// does — the scroll is applied by Draw, from the selection.
	for range 20 {
		mb.HandleKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		mb.DrawOverlay(s)
	}
	if mb.menu.root().hover != 20 {
		t.Fatalf("selected item = %d after 20 Downs over 21 enabled items, want 20", mb.menu.root().hover)
	}
	if mb.menu.root().top == 0 {
		t.Fatalf("scrollTop = 0 with item 20 selected in a %d-row window; "+
			"want the box scrolled so the selection is on screen", rows)
	}
	if mb.menu.root().hover < mb.menu.root().top || mb.menu.root().hover >= mb.menu.root().top+rows {
		t.Fatalf("selected item %d is outside the drawn window [%d,%d)",
			mb.menu.root().hover, mb.menu.root().top, mb.menu.root().top+rows)
	}

	// And it is genuinely reachable, not merely highlighted off-screen.
	mb.HandleKey(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if ran != "Item 20" {
		t.Fatalf("Enter on the last item ran %q, want %q", ran, "Item 20")
	}
}

func TestClickOnAScrolledDropdownHitsTheItemDrawnThere(t *testing.T) {
	var ran string
	mb, s := openTallMenuBar(21, 20, &ran)
	for range 20 {
		mb.HandleKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		mb.DrawOverlay(s)
	}
	top := mb.menu.root().top
	if top == 0 {
		t.Fatal("dropdown did not scroll; the rest of this test proves nothing")
	}
	r := mb.dropdownRect()

	// The box's first content row shows items[scrollTop], not items[0]. A
	// hit-test that forgets the offset runs whatever sits that far down the
	// unscrolled list instead.
	mb.HandleMouse(tcell.NewEventMouse(r.X+2, r.Y+1, tcell.Button1, tcell.ModNone))
	want := fmt.Sprintf("Item %d", top)
	if ran != want {
		t.Fatalf("clicking the dropdown's first drawn row ran %q, want %q", ran, want)
	}
}

func TestWheelScrollsAnOverTallDropdown(t *testing.T) {
	var ran string
	mb, s := openTallMenuBar(21, 20, &ran)
	r := mb.dropdownRect()
	if mb.menu.root().top != 0 {
		t.Fatalf("scrollTop = %d on a freshly opened menu, want 0", mb.menu.root().top)
	}

	for range 3 {
		mb.HandleMouse(tcell.NewEventMouse(r.X+2, r.Y+2, tcell.WheelDown, tcell.ModNone))
		mb.DrawOverlay(s)
	}
	if mb.menu.root().top != 3 {
		t.Fatalf("scrollTop = %d after three wheel-downs, want 3 "+
			"(without the wheel the hidden items are keyboard-only, "+
			"which is no help to the mouse user they were hidden from)", mb.menu.root().top)
	}
	if !mb.OverlayActive() {
		t.Fatal("the wheel closed the dropdown; want it consumed as a scroll")
	}

	for range 5 {
		mb.HandleMouse(tcell.NewEventMouse(r.X+2, r.Y+2, tcell.WheelUp, tcell.ModNone))
		mb.DrawOverlay(s)
	}
	if mb.menu.root().top != 0 {
		t.Fatalf("scrollTop = %d after wheeling back past the top, want 0", mb.menu.root().top)
	}
}

func TestDropdownThatFitsDoesNotScroll(t *testing.T) {
	var ran string
	mb, s := openTallMenuBar(6, 24, &ran)
	for range 5 {
		mb.HandleKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		mb.DrawOverlay(s)
	}
	if mb.menu.root().top != 0 {
		t.Fatalf("scrollTop = %d for a 6-item menu on a 24-row screen, want 0", mb.menu.root().top)
	}
	r := mb.dropdownRect()
	if r.H != 8 {
		t.Fatalf("dropdown height = %d for 6 items, want 8 (six rows and two borders)", r.H)
	}
	// A wheel tick over a menu with nothing to scroll changes nothing.
	mb.HandleMouse(tcell.NewEventMouse(r.X+2, r.Y+1, tcell.WheelDown, tcell.ModNone))
	if mb.menu.root().top != 0 || !mb.OverlayActive() {
		t.Fatalf("top = %d, open = %v after a wheel tick on an unscrollable menu; want 0 and open",
			mb.menu.root().top, mb.OverlayActive())
	}
}

func TestClampMenuScrollKeepsTheSelectionInWindow(t *testing.T) {
	cases := []struct {
		name              string
		top, sel, n, rows int
		want              int
	}{
		{"everything fits", 0, 5, 6, 10, 0},
		{"no rows to draw in", 4, 5, 20, 0, 0},
		{"selection below the window scrolls down", 0, 12, 21, 10, 3},
		{"selection above the window scrolls up", 8, 2, 21, 10, 2},
		{"selection already visible does not move", 5, 7, 21, 10, 5},
		{"a scroll past the end is pulled back", 99, 20, 21, 10, 11},
		{"no selection leaves the window where it is", 4, -1, 21, 10, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := clampMenuScroll(c.top, c.sel, c.n, c.rows)
			if got != c.want {
				t.Fatalf("clampMenuScroll(top=%d, sel=%d, n=%d, rows=%d) = %d, want %d",
					c.top, c.sel, c.n, c.rows, got, c.want)
			}
			if got < 0 {
				t.Fatalf("clampMenuScroll returned a negative index %d", got)
			}
		})
	}
}

func TestContextMenuBiggerThanTheScreenStaysOnIt(t *testing.T) {
	// No shipped context menu is this large, but the shift that fits a menu
	// under the bottom edge goes negative once the menu is taller than the
	// screen, and takes the top items — the ones hover starts on — off it.
	items := make([]MenuItem, 30)
	for i := range items {
		items[i] = MenuItem{Label: fmt.Sprintf("Item %d", i)}
	}
	cm := &ContextMenu{}
	cm.Show(70, 18, items)
	cm.DrawOverlay(tuikittest.NewScreen(20, 10))
	r := cm.drawnRect()
	x, y, w, h := r.X, r.Y, r.W, r.H
	if x < 0 || y < 0 || y+h > 10 {
		t.Fatalf("context menu drawn at (%d,%d) size %dx%d on a 20x10 screen; "+
			"want both coordinates on screen", x, y, w, h)
	}
}

// SI2: a context menu taller than the screen scrolls like a dropdown instead
// of losing its bottom rows; every item is reachable by key and by wheel.
func TestContextMenuTallerThanTheScreenScrolls(t *testing.T) {
	var ran string
	items := tallMenu(30, &ran)[0].Items
	cm := &ContextMenu{}
	cm.Show(2, 2, items)
	s := tuikittest.NewScreen(80, 12)
	cm.DrawOverlay(s)
	r := cm.drawnRect()
	if r.Y != 0 || r.H != 12 {
		t.Fatalf("box = %+v on a 12-row screen, want it shifted to row 0 and cut to 12 rows", r)
	}
	for range 30 { // Down from nothing highlighted lands on 0; 29 more reach the last
		cm.HandleKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
		cm.DrawOverlay(s)
	}
	root := cm.menu.root()
	if root.hover != 29 || root.top != 30-(r.H-2) {
		t.Fatalf("hover = %d, top = %d after walking to the end; want 29 and %d", root.hover, root.top, 30-(r.H-2))
	}
	for range 40 {
		cm.HandleMouse(tcell.NewEventMouse(r.X+2, r.Y+3, tcell.WheelUp, tcell.ModNone))
		cm.DrawOverlay(s)
	}
	if root.top != 0 {
		t.Fatalf("top = %d after wheeling up past the start, want 0", root.top)
	}
	cm.HandleMouse(tcell.NewEventMouse(r.X+2, r.Y+1, tcell.Button1, tcell.ModNone))
	if ran != "Item 0" {
		t.Fatalf("click on the first drawn row ran %q, want %q", ran, "Item 0")
	}
}

// SI2: a submenu taller than the screen scrolls too, and a click on it hits
// the item drawn there rather than the one that far down the unscrolled list.
func TestSubmenuTallerThanTheScreenScrolls(t *testing.T) {
	var ran string
	cm := &ContextMenu{}
	cm.Show(2, 2, []MenuItem{{Label: "Many", Sub: tallMenu(30, &ran)[0].Items}})
	s := tuikittest.NewScreen(80, 12)
	cm.DrawOverlay(s)
	cm.HandleKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
	cm.HandleKey(tcell.NewEventKey(tcell.KeyRight, "", tcell.ModNone))
	cm.DrawOverlay(s)
	sub := cm.menu.boxes[1].rect
	if sub.Y < 0 || sub.Y+sub.H > 12 {
		t.Fatalf("submenu box = %+v on a 12-row screen, want it on screen", sub)
	}
	for range 5 {
		cm.HandleMouse(tcell.NewEventMouse(sub.X+2, sub.Y+2, tcell.WheelDown, tcell.ModNone))
		cm.DrawOverlay(s)
	}
	if top := cm.menu.boxes[1].top; top != 5 {
		t.Fatalf("submenu top = %d after five wheel-downs, want 5", top)
	}
	cm.HandleMouse(tcell.NewEventMouse(sub.X+2, sub.Y+1, tcell.Button1, tcell.ModNone))
	if ran != "Item 5" {
		t.Fatalf("click on the scrolled submenu's first row ran %q, want %q", ran, "Item 5")
	}
}

// OP2: Enabled runs once per item per draw, and not at all on a mouse move
// that only moves the hover over enabled rows' neighbours.
func TestMenuEnabledRunsOncePerItemPerDraw(t *testing.T) {
	calls := 0
	items := make([]MenuItem, 5)
	for i := range items {
		items[i] = MenuItem{Label: fmt.Sprint(i), Note: "why", Enabled: func() bool { calls++; return i != 2 }}
	}
	s := tuikittest.NewScreen(80, 24)

	cm := &ContextMenu{}
	cm.Show(2, 2, items)
	calls = 0
	cm.DrawOverlay(s)
	if calls != len(items) {
		t.Errorf("ContextMenu draw: Enabled called %d times for %d items, want once each", calls, len(items))
	}
	calls = 0
	cm.HandleMouse(tcell.NewEventMouse(30, 20, tcell.ButtonNone, tcell.ModNone)) // outside
	if calls != 0 {
		t.Errorf("motion outside the menu called Enabled %d times, want 0", calls)
	}

	mb := NewMenuBar()
	mb.SetBounds(0, 0, 80)
	mb.SetMenus([]Menu{{Label: "File", Items: items}})
	mb.Open()
	calls = 0
	mb.DrawOverlay(s)
	if calls != len(items) {
		t.Errorf("MenuBar draw: Enabled called %d times for %d items, want once each", calls, len(items))
	}
}
