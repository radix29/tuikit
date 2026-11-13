package dialogs

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// FieldGesture is the drag latch a dialog needs for the gesture a press inside
// one of its controls starts: a text selection in an InputField or Editor, an
// item or thumb drag in a ListBox. Embed one in any dialog hosting such a
// control and drive it from HandleMouse; it is the dialog-level half of the
// control's own latch: a fresh press arms it, the held button's resends
// continue it, and the release ends it. Without it the dialog hit-tests each
// resend, so a drag that began in a list and crossed a text field acts on the
// field as a fresh press.
//
// It is a type, not a per-dialog idiom, because the idiom is correct only in one
// order: it depends on what ModalDialog.ConsumeOutsideClick and a dialog's mode
// switch do to the events the latch needs. See Release and Replay.
type FieldGesture struct {
	field core.MouseHandler
}

// Release ends the gesture, forwarding the release to whichever field claimed
// the press, wherever the pointer is by then.
//
// Call it at the very top of HandleMouse, on ButtonNone, before
// ConsumeOutsideClick and any early return for a dialog mode. Both return
// without looking at the latch, and a release outside the dialog (or during a
// progress view) is what strands it: the field stays latched and its next press
// is swallowed as a continuation of the finished drag.
//
// A no-op unless ev is a release and a gesture is held, so safe to call
// unconditionally.
func (g *FieldGesture) Release(ev *tcell.EventMouse) {
	if ev.Buttons() != tcell.ButtonNone || g.field == nil {
		return
	}
	g.field.HandleMouse(ev)
	g.field = nil
}

// Replay reports whether a gesture is held, forwarding ev to its owner when it
// is; true means the event is spoken for and HandleMouse must return.
//
// Call it after ConsumeOutsideClick and before any hit-testing: hit-testing a
// motion event would end the selection when the pointer left the field's rect,
// and letting it reach ButtonClicked would fire a button when a selection drag
// wandered over the button row.
func (g *FieldGesture) Replay(ev *tcell.EventMouse) bool {
	if g.field == nil || ev.Buttons() != tcell.Button1 {
		return false
	}
	g.field.HandleMouse(ev)
	return true
}

// Claim gives the gesture to f and forwards the press, so the click positions
// the cursor or starts a selection rather than only moving focus. Call it from
// the branch whose hit-test matched f. f should be a core.DragCanceller (InputField,
// ListBox and Editor are), or Clear cannot reach its latch.
func (g *FieldGesture) Claim(f core.MouseHandler, ev *tcell.EventMouse) {
	g.field = f
	f.HandleMouse(ev)
}

// Clear drops a held gesture without forwarding anything, and the control's own
// latch with it (core.DragCanceller). Call it from Show: a latch must not survive into
// the dialog's next showing, or the first press of the new session reads as the
// last one's drag continuing.
//
// Both halves, because a dialog dismissed mid-drag (Escape with the button down)
// never sees the release, and dialogs that build their fields once (Connect,
// Options, Find/Replace, Log Search) hand the same field back next showing. With
// its latch still set, the first press after reopening takes InputField's
// continued-drag branch, placing the cursor without arming a new anchor.
// A latch must not survive into the next showing (ModalDialog.Show clears the
// dialog's own), and this is the only call that can reach the field's. Backup, Restore and Filter rebuild their
// fields in show, discarding the stale one.
func (g *FieldGesture) Clear() {
	if c, ok := g.field.(core.DragCanceller); ok {
		c.CancelMouseDrag()
	}
	g.field = nil
}

// Field is the control holding the gesture, or nil, for a caller or test that
// needs more than "a gesture is in progress".
func (g *FieldGesture) Field() core.MouseHandler { return g.field }
