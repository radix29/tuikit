package core

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

type fakeOverlay struct{ open bool }

func (*fakeOverlay) DrawOverlay(tcell.Screen) {}
func (f *fakeOverlay) OverlayActive() bool    { return f.open }

func TestOpenOverlay(t *testing.T) {
	a, b, c := &fakeOverlay{}, &fakeOverlay{open: true}, &fakeOverlay{open: true}

	if got, ok := OpenOverlay(a, b, c); !ok || got != b {
		t.Errorf("OpenOverlay(a, b, c) = %p, %v; want b (%p), true", got, ok, b)
	}
	if got, ok := OpenOverlay(a); ok || got != nil {
		t.Errorf("OpenOverlay(closed) = %p, %v; want nil, false", got, ok)
	}
	if got, ok := OpenOverlay[*fakeOverlay](); ok || got != nil {
		t.Errorf("OpenOverlay() = %p, %v; want nil, false", got, ok)
	}
	// An interface-typed list answers through the interface.
	if got, ok := OpenOverlay[Overlay](a, c); !ok || got != Overlay(c) {
		t.Errorf("OpenOverlay[Overlay](a, c) = %v, %v; want c, true", got, ok)
	}
}

// fakeMenu is a MouseOverlay whose HandleMouse result and close-on-handle are
// scripted per test.
type fakeMenu struct {
	open, handled, closes bool
	offered               int
}

func (m *fakeMenu) OverlayActive() bool { return m.open }
func (m *fakeMenu) HandleMouse(*tcell.EventMouse) bool {
	m.offered++
	if m.closes {
		m.open = false
	}
	return m.handled
}

func TestRouteOverlay(t *testing.T) {
	ev := tcell.NewEventMouse(3, 3, tcell.Button2, tcell.ModNone)
	for _, c := range []struct {
		name                  string
		open, handled, closes bool
		want                  bool
		offered               int
	}{
		{"closed: not offered, falls through", false, false, false, false, 0},
		{"handled inside the menu", true, true, false, true, 1},
		{"declined, menu still open: spent", true, false, false, true, 1},
		{"press outside closed it and was declined: falls through", true, false, true, false, 1},
		{"press outside closed it and was consumed", true, true, true, true, 1},
	} {
		m := &fakeMenu{open: c.open, handled: c.handled, closes: c.closes}
		if got := RouteOverlay(m, ev); got != c.want || m.offered != c.offered {
			t.Errorf("%s: RouteOverlay = %v (offered %d), want %v (offered %d)", c.name, got, m.offered, c.want, c.offered)
		}
	}
}
