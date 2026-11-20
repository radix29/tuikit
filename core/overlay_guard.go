package core

import "github.com/gdamore/tcell/v3"

// OverlayGuard drops the rest of a held press once the host's overlay layers
// (its top dialog, an open menu, a dropdown) have changed since the press.
// The bug it prevents: a context-menu item opens a dialog on the press, the
// user twitches before releasing, and tcell resends the held Button1 — which
// the new dialog takes as its own fresh press and clicks whatever button is
// under the pointer. Widgets' PressLatch cannot catch it: the dialog never
// saw the press, so the resend is the first Button1 it sees.
//
// S is the host's snapshot of its overlay layers, compared with ==. Its zero
// value holds the zero snapshot.
type OverlayGuard[S comparable] struct{ at S }

// Drop records snap() on a fresh press (fresh is the router's PressLatch
// verdict on ev) and reports false. On a held Button1 it reports whether
// snap() differs from the one recorded, so the router drops the event.
// Anything else — a release, motion, the wheel, another button — reports
// false without calling snap: a release must still reach every latch to
// reset it, and a snapshot on every motion event would be wasted.
//
// Call it before any routing; snap reads the layers as they stand now.
func (g *OverlayGuard[S]) Drop(ev *tcell.EventMouse, fresh bool, snap func() S) bool {
	if fresh {
		g.at = snap()
		return false
	}
	return ev.Buttons() == tcell.Button1 && snap() != g.at
}
