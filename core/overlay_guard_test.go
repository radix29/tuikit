package core

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestOverlayGuard(t *testing.T) {
	ev := func(b tcell.ButtonMask) *tcell.EventMouse { return tcell.NewEventMouse(1, 1, b, tcell.ModNone) }
	type layers struct {
		dialog int
		menu   bool
	}
	var g OverlayGuard[layers]
	var l PressLatch
	step := func(b tcell.ButtonMask, now layers) bool {
		e := ev(b)
		l.Release(e)
		calls := 0
		drop := g.Drop(e, l.Fresh(e), func() layers { calls++; return now })
		if b != tcell.Button1 && calls != 0 {
			t.Fatalf("buttons %v: snap called on a non-Button1 event", b)
		}
		return drop
	}

	menu := layers{menu: true}
	if step(tcell.Button1, menu) {
		t.Fatal("the fresh press was dropped")
	}
	if step(tcell.Button1, menu) {
		t.Fatal("a resend with the layers unchanged was dropped")
	}
	// The menu item, acting on the press, opened a dialog: the twitch's resend
	// must not reach it.
	dlg := layers{dialog: 1}
	if !step(tcell.Button1, dlg) {
		t.Fatal("a resend after the layers changed was kept")
	}
	if step(tcell.WheelDown, dlg) || step(tcell.Button2, dlg) {
		t.Fatal("a non-Button1 event was dropped")
	}
	if step(tcell.ButtonNone, dlg) {
		t.Fatal("the release was dropped; latches would stay armed")
	}
	// The next gesture starts against the dialog.
	if step(tcell.Button1, dlg) {
		t.Fatal("the next gesture's press was dropped")
	}
	if step(tcell.Button1, dlg) {
		t.Fatal("the next gesture's resend was dropped")
	}
}
