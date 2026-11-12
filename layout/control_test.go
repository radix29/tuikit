package layout

import (
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/tuikittest"
)

// Which of core's control interfaces each layout type implements; see the
// widgets package's control_test.go for why these are pinned. PanelManager is
// not a Focusable: its panels are, through Activatable.
var (
	_ interface {
		core.Focusable
		core.Bounded
		core.KeyHandler
		core.MouseHandler
	} = (*Splitter)(nil)
	_ interface {
		core.Bounded
		core.KeyHandler
		core.MouseHandler
	} = (*PanelManager)(nil)
)

// The contract tests below (AR2) run every layout type through the rules of
// core.MouseHandler and core.KeyHandler and tuikit's ARCHITECTURE.md § Mouse
// gestures; controls/control_test.go has the same tables for the controls.

type contractLayout interface {
	core.KeyHandler
	core.MouseHandler
	SetBounds(x, y, w, h int)
	Draw(tcell.Screen)
}

func contractLayouts() []struct {
	name string
	l    contractLayout
} {
	pm := NewPanelManager()
	for _, title := range []string{"Query 1", "Query 2", "Details"} {
		pm.AddPanel(&fakePanel{title: title, closable: true})
	}
	ls := []struct {
		name string
		l    contractLayout
	}{
		{"Splitter vertical", NewSplitter(SplitterVertical, 0.5)},
		{"Splitter horizontal", NewSplitter(SplitterHorizontal, 0.5)},
		{"PanelManager", pm},
	}
	for _, c := range ls {
		c.l.SetBounds(0, 0, 40, 10)
	}
	return ls
}

// Plain motion is never claimed, anywhere over a layout type, the splitter
// bar and the tab row included.
func TestContractHoverIsNotClaimed(t *testing.T) {
	for _, c := range contractLayouts() {
		for y := range 10 {
			for x := range 40 {
				if c.l.HandleMouse(tuikittest.Mouse(x, y, tcell.ButtonNone)) {
					t.Errorf("%s: hover at (%d,%d) claimed", c.name, x, y)
				}
			}
		}
	}
}

// A release anywhere ends every gesture: afterwards neither plain motion nor
// a fresh press off the bar and the tab row is claimed.
func TestContractReleaseAnywhereClearsEveryLatch(t *testing.T) {
	press := map[string][][2]int{
		"Splitter vertical":   {{20, 5}},                 // the bar
		"Splitter horizontal": {{10, 5}},                 // the bar
		"PanelManager":        {{2, 0}, {38, 0}, {9, 5}}, // a tab, the combo arrow, content
	}
	for _, outside := range []bool{false, true} {
		for i := range contractLayouts() {
			name := contractLayouts()[i].name
			for _, p := range press[name] {
				l := contractLayouts()[i].l
				rel := p
				if outside {
					rel = [2]int{70, 30}
				}
				l.HandleMouse(tuikittest.Press(p[0], p[1]))
				l.HandleMouse(tuikittest.Mouse(p[0]+1, p[1]+1, tcell.Button1))
				l.HandleMouse(tuikittest.Release(rel[0], rel[1]))
				if pm, ok := l.(*PanelManager); ok && pm.comboOpen {
					continue // the press opened the drop-down: the overlay rules apply
				}
				if l.HandleMouse(tuikittest.Mouse(5, 7, tcell.ButtonNone)) {
					t.Errorf("%s: press at %v, release at %v: next motion claimed", name, p, rel)
				}
				if l.HandleMouse(tuikittest.Press(70, 30)) {
					t.Errorf("%s: press at %v, release at %v: a later press outside claimed", name, p, rel)
				}
			}
		}
	}
}

// Navigation keys on a PanelManager with no panels, sized or not, neither
// panic nor break the next Draw.
func TestContractNavKeysOnEmptyContentKeepDrawWorking(t *testing.T) {
	keys := []tcell.Key{
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn,
		tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight, tcell.KeyEnter,
	}
	for _, sized := range []bool{true, false} {
		for _, mod := range []tcell.ModMask{tcell.ModNone, tcell.ModShift, tcell.ModCtrl, tcell.ModAlt} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("PanelManager (sized %v, mod %v): panic: %v", sized, mod, r)
					}
				}()
				pm := NewPanelManager()
				if sized {
					pm.SetBounds(0, 0, 40, 10)
				}
				scr := tuikittest.NewScreen(40, 10)
				for _, k := range keys {
					pm.HandleKey(tuikittest.Key(k, mod))
					pm.Draw(scr)
				}
			}()
		}
	}
}
