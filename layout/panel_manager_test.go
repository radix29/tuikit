package layout

import (
	"slices"
	"testing"

	"github.com/gdamore/tcell/v3"
)

// fakePanel is a minimal Panel (+ optionally Closable) for PanelManager
// tests that don't need real rendering or input handling.
type fakePanel struct {
	title    string
	closable bool

	// mouse records the button state of every event the panel manager
	// actually routed through, so a test can assert what did not reach the
	// panel as well as what did.
	mouse []tcell.ButtonMask
}

func (p *fakePanel) SetBounds(x, y, w, h int)          {}
func (p *fakePanel) Draw(s tcell.Screen)               {}
func (p *fakePanel) HandleKey(ev *tcell.EventKey) bool { return false }
func (p *fakePanel) HandleMouse(ev *tcell.EventMouse) bool {
	p.mouse = append(p.mouse, ev.Buttons())
	return false
}
func (p *fakePanel) Title() string  { return p.title }
func (p *fakePanel) Closable() bool { return p.closable }

// TestPanelManagerNonClosablePanelNeverFiresOnCloseTab confirms a panel
// that implements Closable and returns false (e.g. Object Explorer
// Details) can never trigger OnCloseTab via a tab-bar click — Draw skips
// drawing its [x] entirely (closeW == 0), and HandleMouse's hit-testing
// walk must skip the close-button check the same way, or a click landing
// where the glyph would have been for a closable panel could still close
// this one.
func TestPanelManagerNonClosablePanelNeverFiresOnCloseTab(t *testing.T) {
	pm := NewPanelManager()
	pm.AddPanel(&fakePanel{title: "Object Explorer Details", closable: false})
	pm.AddPanel(&fakePanel{title: "Query 1", closable: true})
	pm.SetBounds(0, 0, 80, 24)

	closed := -1
	pm.OnCloseTab = func(i int) { closed = i }

	for x := range 40 {
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone))
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone))
	}
	if closed == 0 {
		t.Fatalf("OnCloseTab fired for panel 0, which is non-closable")
	}
}

// TestPanelManagerClosablePanelStillClosesViaClick is the mirror check:
// a panel that doesn't implement Closable (the common case, e.g. a query
// editor) must still close normally by clicking its [x].
func TestPanelManagerClosablePanelStillClosesViaClick(t *testing.T) {
	pm := NewPanelManager()
	pm.AddPanel(&fakePanel{title: "Object Explorer Details", closable: false})
	pm.AddPanel(&fakePanel{title: "Query 1", closable: true})
	pm.SetBounds(0, 0, 80, 24)

	closed := -1
	pm.OnCloseTab = func(i int) { closed = i }

	// Each x is a separate fresh press (release first) — the press latch
	// would otherwise treat this sweep as one held drag and swallow every
	// position after the first, since it has no bearing on cursor motion
	// speed/distance, only on whether a release was seen since the last
	// press.
	found := false
	for x := 0; x < 60 && !found; x++ {
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone))
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone))
		if closed == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("OnCloseTab never fired for panel 1, which is closable")
	}
}

// TestPanelManagerHeldButtonOnCloseDoesNotReFire covers tcell's all-motion
// mouse tracking resending Buttons()==Button1 on every cursor motion while
// the button stays down: without the press latch a single physical
// click on a tab's [x] that twitches fires OnCloseTab twice, stacking
// duplicate "save this Dirty panel?" prompts for one click.
func TestPanelManagerHeldButtonOnCloseDoesNotReFire(t *testing.T) {
	pm := NewPanelManager()
	pm.AddPanel(&fakePanel{title: "Object Explorer Details", closable: false})
	pm.AddPanel(&fakePanel{title: "Query 1", closable: true})
	pm.SetBounds(0, 0, 80, 24)

	closeCount := 0
	pm.OnCloseTab = func(i int) {
		if i == 1 {
			closeCount++
		}
	}

	closeX := -1
	for x := 0; x < 60 && closeX < 0; x++ {
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone))
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone))
		if closeCount == 1 {
			closeX = x
		}
	}
	if closeX < 0 {
		t.Fatal("could not locate panel 1's close button to set up the test")
	}

	// A resent Button1 at the same spot, with no release in between, must
	// not fire OnCloseTab again.
	pm.HandleMouse(tcell.NewEventMouse(closeX, 0, tcell.Button1, tcell.ModNone))
	if closeCount != 1 {
		t.Fatalf("closeCount after resent Button1 (no release) = %d, want 1 (still the same physical press)", closeCount)
	}
}

// fakePanelNoClosable is a Panel that doesn't implement Closable at all —
// used to check PanelClosable's default (see TestPanelClosableDefaultsTrueWithoutInterface).
type fakePanelNoClosable struct{ title string }

func (p fakePanelNoClosable) SetBounds(x, y, w, h int)              {}
func (p fakePanelNoClosable) Draw(s tcell.Screen)                   {}
func (p fakePanelNoClosable) HandleKey(ev *tcell.EventKey) bool     { return false }
func (p fakePanelNoClosable) HandleMouse(ev *tcell.EventMouse) bool { return false }
func (p fakePanelNoClosable) Title() string                         { return p.title }

func TestPanelClosableDefaultsTrueWithoutInterface(t *testing.T) {
	if !PanelClosable(fakePanelNoClosable{title: "x"}) {
		t.Fatalf("PanelClosable = false for a panel not implementing Closable, want true (default)")
	}
	if PanelClosable(&fakePanel{title: "x", closable: false}) {
		t.Fatalf("PanelClosable = true for Closable() == false")
	}
	if !PanelClosable(&fakePanel{title: "x", closable: true}) {
		t.Fatalf("PanelClosable = false for Closable() == true")
	}
}

// activatablePanel is a fakePanel that also implements Activatable, so
// RemovePanel tests can assert exactly which panel(s) got a SetActive call.
type activatablePanel struct {
	fakePanel
	active bool
}

func (p *activatablePanel) SetActive(v bool) { p.active = v }

// TestPanelManagerRemovePanelKeepsSamePanelActiveWhenRemovingBeforeIt
// removes a panel to the left of the active one: the active panel's index
// must shift down by one so the same panel stays active, rather than
// pm.active keeping its old numeric value and pointing at whichever panel
// now occupies that slot.
func TestPanelManagerRemovePanelKeepsSamePanelActiveWhenRemovingBeforeIt(t *testing.T) {
	pm := NewPanelManager()
	a := &activatablePanel{title: "A"}
	b := &activatablePanel{title: "B"}
	c := &activatablePanel{title: "C"}
	d := &activatablePanel{title: "D"}
	pm.AddPanel(a)
	pm.AddPanel(b)
	pm.AddPanel(c)
	pm.AddPanel(d)
	pm.SetActiveIndex(2) // C
	c.active = true

	pm.RemovePanel(0) // remove A, to the left of active

	if got := pm.ActivePanel(); got != Panel(c) {
		t.Fatalf("ActivePanel() = %v, want C to remain active", got)
	}
	if pm.ActiveIndex() != 1 {
		t.Fatalf("ActiveIndex() = %d, want 1 (C shifted down after A's removal)", pm.ActiveIndex())
	}
	if d.active {
		t.Fatalf("SetActive(true) fired on D, which was never made active")
	}
}

// TestPanelManagerRemovePanelActivatesNeighborWhenActiveIsRemoved covers
// removing the active panel itself: a neighbor must become active and get
// a SetActive(true) call.
func TestPanelManagerRemovePanelActivatesNeighborWhenActiveIsRemoved(t *testing.T) {
	pm := NewPanelManager()
	a := &activatablePanel{title: "A"}
	b := &activatablePanel{title: "B"}
	c := &activatablePanel{title: "C"}
	pm.AddPanel(a)
	pm.AddPanel(b)
	pm.AddPanel(c)
	pm.SetActiveIndex(1) // B
	b.active = true

	pm.RemovePanel(1) // remove B, the active panel

	if got := pm.ActivePanel(); got != Panel(c) {
		t.Fatalf("ActivePanel() = %v, want C (index 1 after removal)", got)
	}
	if !c.active {
		t.Fatalf("SetActive(true) never fired on C after it became active")
	}
}

// TestPanelManagerRemovePanelAfterActiveDoesNotChangeActive removes a
// panel to the right of the active one, which shouldn't move the active
// index at all.
func TestPanelManagerRemovePanelAfterActiveDoesNotChangeActive(t *testing.T) {
	pm := NewPanelManager()
	a := &activatablePanel{title: "A"}
	b := &activatablePanel{title: "B"}
	c := &activatablePanel{title: "C"}
	pm.AddPanel(a)
	pm.AddPanel(b)
	pm.AddPanel(c)
	pm.SetActiveIndex(1) // B
	b.active = true

	pm.RemovePanel(2) // remove C, to the right of active

	if got := pm.ActivePanel(); got != Panel(b) {
		t.Fatalf("ActivePanel() = %v, want B to remain active", got)
	}
	if pm.ActiveIndex() != 1 {
		t.Fatalf("ActiveIndex() = %d, want 1 (unchanged)", pm.ActiveIndex())
	}
}

// openerPanel is a fakePanel whose first Button1 opens another panel and
// switches to it, as a panel's own button does (gossms's Execution Plan
// "[ Expand ]").
type openerPanel struct {
	fakePanel
	pm     *PanelManager
	opened *fakePanel
}

func (p *openerPanel) HandleMouse(ev *tcell.EventMouse) bool {
	p.fakePanel.HandleMouse(ev)
	if ev.Buttons() == tcell.Button1 && p.opened == nil {
		p.opened = &fakePanel{title: "Opened"}
		p.pm.SetActiveIndex(p.pm.AddPanel(p.opened))
	}
	return true
}

// TestPanelManagerPressThatSwitchesPanelsStaysWithItsPanel: a press that opens
// and activates another panel, then drags, must not
// hand its resends to the new panel (a press it never saw begin), and the
// release must still reach the opener so its press latch resets.
func TestPanelManagerPressThatSwitchesPanelsStaysWithItsPanel(t *testing.T) {
	pm := NewPanelManager()
	op := &openerPanel{fakePanel: fakePanel{title: "Query 1"}, pm: pm}
	pm.AddPanel(op)
	pm.SetBounds(0, 0, 80, 24)

	pm.HandleMouse(tcell.NewEventMouse(10, 5, tcell.Button1, tcell.ModNone))
	if op.opened == nil || pm.ActivePanel() != Panel(op.opened) {
		t.Fatal("setup: the press did not open and activate the new panel")
	}
	pm.HandleMouse(tcell.NewEventMouse(11, 6, tcell.Button1, tcell.ModNone))
	if len(op.opened.mouse) != 0 {
		t.Fatalf("new panel got %v mid-gesture, want nothing", op.opened.mouse)
	}

	pm.HandleMouse(tcell.NewEventMouse(11, 6, tcell.ButtonNone, tcell.ModNone))
	if want := []tcell.ButtonMask{tcell.Button1, tcell.ButtonNone}; !slices.Equal(op.mouse, want) {
		t.Fatalf("opener got %v, want %v (press, then the release)", op.mouse, want)
	}
	if want := []tcell.ButtonMask{tcell.ButtonNone}; !slices.Equal(op.opened.mouse, want) {
		t.Fatalf("new panel got %v, want %v (only the release)", op.opened.mouse, want)
	}

	// The next press is a fresh gesture on the new panel.
	pm.HandleMouse(tcell.NewEventMouse(10, 5, tcell.Button1, tcell.ModNone))
	if n := len(op.opened.mouse); n != 2 || op.opened.mouse[1] != tcell.Button1 {
		t.Fatalf("new panel got %v, want its next press delivered", op.opened.mouse)
	}
}

// tabLabelX finds an x on row 0 whose click switches to panel i, by sweeping
// with release/press pairs on a throwaway manager laid out like pm.
func tabLabelX(t *testing.T, mk func() *PanelManager, i int) int {
	t.Helper()
	for x := 0; x < 60; x++ {
		pm := mk()
		pm.HandleMouse(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone))
		if pm.ActiveIndex() == i {
			return x
		}
	}
	t.Fatalf("no x on the tab row switches to panel %d", i)
	return -1
}

// TestPanelManagerDragFromContentOntoTabRowIsNotAClick: a press that began in
// the active panel, dragged up over the tab row, is the panel's drag. It must
// not switch tabs, close one, or toggle the drop-down; the panel keeps getting it.
func TestPanelManagerDragFromContentOntoTabRowIsNotAClick(t *testing.T) {
	a := &fakePanel{title: "a", closable: true}
	mk := func() *PanelManager {
		pm := NewPanelManager()
		pm.AddPanel(&fakePanel{title: "a", closable: true})
		pm.AddPanel(&fakePanel{title: "b", closable: true})
		pm.SetBounds(0, 0, 80, 24)
		return pm
	}
	bx := tabLabelX(t, mk, 1)

	pm := NewPanelManager()
	pm.AddPanel(a)
	pm.AddPanel(&fakePanel{title: "b", closable: true})
	pm.SetBounds(0, 0, 80, 24)
	closed := -1
	pm.OnCloseTab = func(i int) { closed = i }

	b1 := func(x, y int) { pm.HandleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)) }
	b1(10, 5)
	b1(bx, 0)   // over tab b
	b1(2, 0)    // over tab a, possibly its [x]
	b1(80-2, 0) // over the combo arrow
	if pm.ActiveIndex() != 0 || closed != -1 || pm.comboOpen {
		t.Fatalf("drag read as chrome click: active=%d closed=%d comboOpen=%v", pm.ActiveIndex(), closed, pm.comboOpen)
	}
	if len(a.mouse) != 4 {
		t.Fatalf("panel got %d events, want all 4", len(a.mouse))
	}

	// Release, then a fresh press on b switches: the latch recovers.
	pm.HandleMouse(tcell.NewEventMouse(bx, 0, tcell.ButtonNone, tcell.ModNone))
	b1(bx, 0)
	if pm.ActiveIndex() != 1 {
		t.Fatalf("fresh press on tab b after release: active=%d, want 1", pm.ActiveIndex())
	}
}

// tabStripFixture is 12 closable "Query N" panels in an 80-column manager.
func tabStripFixture() *PanelManager {
	pm := NewPanelManager()
	for i := 1; i <= 12; i++ {
		pm.AddPanel(&fakePanel{title: "Query " + string(rune('A'+i-1)), closable: true})
	}
	pm.SetBounds(0, 0, 80, 24)
	return pm
}

// TestPanelManagerActiveTabStaysInStrip: the strip follows the active tab, so
// the active panel always has a segment, a click on it keeps it active, and
// moving back to 0 scrolls back.
func TestPanelManagerActiveTabStaysInStrip(t *testing.T) {
	pm := tabStripFixture()
	if _, segs := pm.tabStrip(); len(segs) >= pm.Count() {
		t.Fatalf("fixture too narrow to overflow: %d of %d tabs fit", len(segs), pm.Count())
	}
	for _, want := range []int{9, 11, 5, 0, 10} {
		pm.SetActiveIndex(want)
		first, segs := pm.tabStrip()
		if want < first || want >= first+len(segs) {
			t.Fatalf("active %d outside strip [%d,%d)", want, first, first+len(segs))
		}
		seg := segs[want-first][0]
		pm.HandleMouse(tcell.NewEventMouse(seg.X, 0, tcell.ButtonNone, tcell.ModNone))
		pm.HandleMouse(tcell.NewEventMouse(seg.X, 0, tcell.Button1, tcell.ModNone))
		pm.HandleMouse(tcell.NewEventMouse(seg.X, 0, tcell.ButtonNone, tcell.ModNone))
		if pm.ActiveIndex() != want {
			t.Fatalf("click on active tab %d switched to %d", want, pm.ActiveIndex())
		}
	}
	pm.SetActiveIndex(0)
	if first, _ := pm.tabStrip(); first != 0 {
		t.Fatalf("after returning to 0 the strip starts at %d", first)
	}
}

// TestPanelManagerScrolledStripClosesRightTab: hit-testing indexes from the
// strip's first panel, not from 0.
func TestPanelManagerScrolledStripClosesRightTab(t *testing.T) {
	pm := tabStripFixture()
	pm.SetActiveIndex(9)
	first, segs := pm.tabStrip()
	if first == 0 {
		t.Fatal("strip did not scroll")
	}
	closed := -1
	pm.OnCloseTab = func(i int) { closed = i }
	cs := segs[9-first][1]
	pm.HandleMouse(tcell.NewEventMouse(cs.X, 0, tcell.Button1, tcell.ModNone))
	if closed != 9 {
		t.Fatalf("[x] of panel 9 closed %d", closed)
	}
}

// TestPanelManagerStripFollowsRemoveAndResize: removing the active last tab
// and narrowing the bar both keep the active tab drawn.
func TestPanelManagerStripFollowsRemoveAndResize(t *testing.T) {
	pm := tabStripFixture()
	pm.SetActiveIndex(11)
	pm.RemovePanel(11)
	first, segs := pm.tabStrip()
	if a := pm.ActiveIndex(); a != 10 || a < first || a >= first+len(segs) {
		t.Fatalf("after remove: active %d, strip [%d,%d)", a, first, first+len(segs))
	}
	pm.SetBounds(0, 0, 40, 24)
	first, segs = pm.tabStrip()
	if a := pm.ActiveIndex(); a < first || a >= first+len(segs) {
		t.Fatalf("after narrowing: active %d, strip [%d,%d)", a, first, first+len(segs))
	}
	pm.SetBounds(0, 0, 400, 24)
	if first, segs = pm.tabStrip(); first != 0 || len(segs) != pm.Count() {
		t.Fatalf("after widening: strip [%d,%d) of %d", first, first+len(segs), pm.Count())
	}
}
