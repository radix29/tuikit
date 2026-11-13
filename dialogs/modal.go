package dialogs

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// ---------------------------------------------------------------------------
// ModalDialog: base type
// ---------------------------------------------------------------------------

// ModalDialog is the foundation for every pop-up window. Embed it and call
// InitModal from the constructor.
type ModalDialog struct {
	rect       core.Rect
	reqW, reqH int // last size passed to InitModal/SetSize, pre-clamp
	title      string
	visible    bool
	screen     tcell.Screen // needed for Size() during recentre

	// press is held from the first Button1 of a gesture until its release,
	// wherever that press landed, and fresh says whether the event being routed
	// is that first Button1 (see TrackPress). The button row fires only on a fresh
	// press: a latch set only by a button hit would stop just the resends of the
	// press that hit it, so a drag that began on the message, a list, a scrollbar
	// or a checkbox would fire whatever button it reached. Released in TrackPress,
	// which every embedding dialog reaches through ConsumeOutsideClick (some reach
	// ButtonClicked only through a mode-gated branch a release never takes); and
	// cleared in Show, for a gesture whose release never gets that far.
	press   core.PressLatch
	fresh   bool
	tracked *tcell.EventMouse // the event press and fresh were last updated for

	// sbDragging is true while a content scrollbar an embedding dialog draws is
	// dragged (see ScrollbarDrag); separate from press, reset alongside it in
	// both places.
	sbDragging bool
}

// InitModal sets up the dialog for the given screen, title and size. Call it
// from the embedding type's constructor.
func (d *ModalDialog) InitModal(s tcell.Screen, title string, w, h int) {
	d.screen = s
	d.title = title
	d.reqW, d.reqH = w, h
	d.recentre()
}

// Screen is the screen InitModal was given (nil for a dialog built without
// one, as tests do) — what an embedding type sizes itself against.
func (d *ModalDialog) Screen() tcell.Screen { return d.screen }

// SetSize resizes the dialog and recentres it. A dialog whose content grows with
// the screen (see propsheet.PropertySheet) calls it from Show or Draw.
func (d *ModalDialog) SetSize(w, h int) {
	d.reqW, d.reqH = w, h
	d.recentre()
}

// recentre centres the dialog, clamping its size to the terminal first (else the
// right/bottom border and anything docked to it draws off-screen). The clamp is
// recomputed from reqW/reqH, not applied to rect.W/H in place, so a dialog
// shown on a cramped terminal returns to full size on a larger one.
func (d *ModalDialog) recentre() {
	if d.screen == nil {
		return
	}
	sw, sh := d.screen.Size()
	d.rect.W = min(d.reqW, sw)
	d.rect.H = min(d.reqH, sh)
	d.rect.X = max(0, (sw-d.rect.W)/2)
	d.rect.Y = max(0, (sh-d.rect.H)/2)
}

// Show makes the dialog visible, recentred.
//
// The drag latches start clear on every showing because their clearing release
// never arrives: a button click closes the dialog on the press, and by the
// matching ButtonNone HandleMouse returns early on !visible and the host has
// dropped the dialog from input routing. A stale latch makes ButtonClicked
// refuse the first click on every reopening. A dialog opening during a held
// gesture is the host's hazard: it must not hand the dialog the rest of a press
// that began before the dialog opened.
func (d *ModalDialog) Show() {
	d.recentre()
	d.press.Clear()
	d.fresh, d.tracked = false, nil
	d.sbDragging = false
	d.visible = true
}

// Relayout re-fits the dialog to the current screen size. The host calls it on
// every terminal resize for each open dialog: recentre otherwise runs only from
// InitModal/SetSize/Show, so the dialog would keep its old rect, drawing its
// border and button row off-screen while still swallowing every key. A dialog
// whose size depends on the screen overrides this to recompute and call SetSize.
func (d *ModalDialog) Relayout() { d.recentre() }

// Hide dismisses the dialog.
func (d *ModalDialog) Hide() { d.visible = false }

// Visible reports whether the dialog is currently shown.
func (d *ModalDialog) Visible() bool { return d.visible }

// Rect returns the dialog's bounding rectangle.
func (d *ModalDialog) Rect() core.Rect { return d.rect }

// SetTitle updates the dialog title.
func (d *ModalDialog) SetTitle(t string) { d.title = t }

// Title is the text SetTitle (or InitModal) last set.
func (d *ModalDialog) Title() string { return d.title }

// ContainsMouse reports whether (mx,my) is inside the dialog box.
func (d *ModalDialog) ContainsMouse(mx, my int) bool {
	return d.rect.Contains(mx, my)
}

// ConsumeOutsideClick returns true if the mouse event originated outside the
// dialog (always visible when called). It calls TrackPress first, whatever the
// position: every embedding dialog's HandleMouse makes this call
// unconditionally, including for a mouse-up outside the rect.
func (d *ModalDialog) ConsumeOutsideClick(ev *tcell.EventMouse) bool {
	d.TrackPress(ev)
	if !d.visible {
		return false
	}
	mx, my := ev.Position()
	return !d.rect.Contains(mx, my)
}

// TrackPress records where ev stands in the current gesture: a release clears
// the press and scrollbar latches, and a Button1 is fresh only if no press is
// held. ConsumeOutsideClick and ButtonClicked call it, so a dialog needs it
// only when it routes events ahead of ConsumeOutsideClick, such as an open
// drop-down list that can reach past the box: call it first in HandleMouse, or
// a press the list consumed is unseen, and its resends, dragged onto the
// button row, look fresh. Repeat calls for one event are no-ops, which is what
// lets every entry point call it.
func (d *ModalDialog) TrackPress(ev *tcell.EventMouse) {
	if ev == d.tracked {
		return
	}
	d.tracked = ev
	if d.press.Release(ev) {
		d.sbDragging = false
	}
	d.fresh = d.press.Fresh(ev)
}

// ScrollbarDrag handles a click or drag on a vertical scrollbar an embedding
// dialog draws at trackX (normally Rect().Right()-1) spanning [trackY,
// trackY+trackH), the counterpart of core.DrawScrollbar. Returns true and updates
// *scroll for a Button1 press on the bar or any continuation of a started drag
// regardless of x. Call it before the dialog's row hit-testing: the bar can sit
// over a column that would resolve to a content row.
func (d *ModalDialog) ScrollbarDrag(ev *tcell.EventMouse, trackX, trackY, trackH, total int, scroll *int) bool {
	return core.HandleScrollbarDrag(ev, trackX, trackY, trackH, total, &d.sbDragging, scroll)
}

// DrawBase fades the underlying UI and draws the dialog box. Embedding types
// call it first, then render their own content within InnerRect().
//
// Clipping needs s to be a *core.ClipScreen (Stack.Draw supplies one). On one,
// DrawBase first widens the clip to the whole screen, so a clip left by a
// dialog drawn earlier on the same ClipScreen never confines this one's dim or
// box, and then narrows it to the interior when the dialog is clamped. On a
// plain tcell.Screen a clamped dialog's content is not clipped.
func (d *ModalDialog) DrawBase(s tcell.Screen) {
	if c, ok := s.(*core.ClipScreen); ok {
		c.ResetClip()
	}
	p := theme.Active()

	// Fade the drawn UI in place, not a solid overlay, so it stays visible dimmed.
	core.DimBehindPopup(s, p.DialogOverlay)

	// Dialog background (opaque, over the dimmed UI)
	core.FillRect(s, d.rect, ' ', theme.StyleDialog())

	// Border + title
	borderStyle := theme.On(p.DialogBg, p.DialogBorder)
	titleStyle := theme.On(p.DialogBg, p.DialogTitle).Bold(true)
	core.DrawBoxTitle(s, d.rect, d.title, borderStyle, titleStyle)

	// Everything the embedding type draws from here is confined to the box (see
	// contentClip); set after the dim and border, which draw outside it.
	if d.clamped() {
		core.SetClip(s, d.contentClip())
	}
}

// clamped reports whether recentre had to shrink the dialog. Embedding types lay
// content out at fixed offsets for the size they asked for, so on a clamped rect
// rows run past the right border and button row, and DrawButtons lands on top.
// The clip and button-row clear are gated on this: a dropdown or completion
// overlay inside a dialog may legitimately extend beyond the box.
func (d *ModalDialog) clamped() bool {
	return d.rect.W < d.reqW || d.rect.H < d.reqH
}

// contentClip is the region an embedding type's content may draw in: the
// interior, border intact. The one exception is its scrollbar on the right
// border column, for which DrawContentScrollbar widens the clip.
func (d *ModalDialog) contentClip() core.Rect { return d.InnerRect() }

// DrawContentScrollbar draws the dialog's vertical scrollbar spanning [trackY,
// trackY+trackH) on the right border column, where ScrollbarDrag hit-tests. That
// column is outside contentClip, so the clip widens to the whole box for the
// draw; core.DrawScrollbar called directly loses its bar on a clamped rect.
func (d *ModalDialog) DrawContentScrollbar(s tcell.Screen, trackY, trackH, total, offset int) {
	if c, ok := s.(*core.ClipScreen); ok {
		saved := c.Clip()
		c.SetClip(d.rect)
		defer c.SetClip(saved)
	}
	sbStyle := theme.StyleDialogLine()
	sbThumb := theme.StyleScrollThumb()
	core.DrawScrollbar(s, d.rect.Right()-1, trackY, trackH, total, trackH, offset, sbStyle, sbThumb)
}

// InnerRect returns the usable interior rectangle (excluding border).
func (d *ModalDialog) InnerRect() core.Rect { return d.rect.Inner(1) }

// ButtonRowY returns the y coordinate of the standard button row, two rows from
// the bottom, above the border.
func (d *ModalDialog) ButtonRowY() int { return d.rect.Y + d.rect.H - 3 }

// DrawSeparator draws a horizontal line one row above ButtonRowY.
func (d *ModalDialog) DrawSeparator(s tcell.Screen) {
	sep := theme.StyleDialogLine()
	core.DrawHLine(s, d.rect.X+1, d.ButtonRowY()-1, d.rect.W-2, sep)
}

// ButtonRowStartX returns the x column the button row starts at so it ends flush
// with the right margin. Shared by DrawButtons and ButtonClicked so hit-testing
// matches the drawing; exported so a dialog drawing status at the row's left end
// can stop short of the buttons.
func (d *ModalDialog) ButtonRowStartX(labels []string) int {
	return d.rect.Right() - 2 - d.ButtonRowWidth(labels)
}

// ButtonRowWidth is how many columns a button row occupies, gaps included.
func (d *ModalDialog) ButtonRowWidth(labels []string) int {
	total := 0
	for i, label := range labels {
		if i > 0 {
			total += 2
		}
		total += core.DisplayWidth("[ " + label + " ]")
	}
	return total
}

// DrawButtons renders a row of buttons at ButtonRowY, right-aligned within
// the dialog. activeIdx highlights that button.
func (d *ModalDialog) DrawButtons(s tcell.Screen, labels []string, activeIdx int) {
	d.DrawButtonsGated(s, labels, activeIdx, nil)
}

// DrawButtonsGated draws the button row with every button whose index is true in
// disabled in the disabled foreground (disabled may be nil or shorter). Drawing
// it gated does not gate it: the handler must still refuse the button.
func (d *ModalDialog) DrawButtonsGated(s tcell.Screen, labels []string, activeIdx int, disabled []bool) {
	d.clearButtonRows(s)
	d.drawButtonGroup(s, d.ButtonRowStartX(labels), d.ButtonRowY(), labels, activeIdx, disabled)
}

// clearButtonRows blanks the button row and the row below it on a clamped rect.
// Content laid out for the full height reaches them; the buttons are
// right-aligned, so without the clear a content row's tail shows with a button
// row in the middle. A dialog with a button row draws nothing of its own from
// ButtonRowY down — which is also what lets a split row's left group drop to the
// row below (see SplitButtonLayout).
func (d *ModalDialog) clearButtonRows(s tcell.Screen) {
	if !d.clamped() {
		return
	}
	y := d.ButtonRowY()
	core.FillRect(s, core.Rect{X: d.rect.X + 1, Y: y, W: d.rect.W - 2, H: d.rect.Bottom() - 1 - y}, ' ', theme.StyleDialog())
}

// drawButtonGroup draws one group of buttons starting at (x, y).
func (d *ModalDialog) drawButtonGroup(s tcell.Screen, x, y int, labels []string, activeIdx int, disabled []bool) {
	p := theme.Active()
	disabledStyle := theme.On(p.ButtonBg, p.TextDisabled)
	btnStyle := theme.StyleButton()
	activeStyle := theme.StyleButtonActive()
	col := x
	for i, label := range labels {
		st := btnStyle
		if i == activeIdx {
			st = activeStyle
		}
		if i < len(disabled) && disabled[i] {
			st = disabledStyle
		}
		text := "[ " + label + " ]"
		core.DrawText(s, col, y, st, text)
		col += core.DisplayWidth(text) + 2
	}
}

// ButtonClicked returns the index of the button clicked, or -1. Only the first
// Button1 of a gesture can click (see TrackPress): not a resend of a held
// click, and not a drag that began elsewhere and reached the button row.
func (d *ModalDialog) ButtonClicked(ev *tcell.EventMouse, labels []string) int {
	d.TrackPress(ev)
	return d.buttonGroupHit(ev, d.ButtonRowStartX(labels), d.ButtonRowY(), labels)
}

// buttonGroupHit is the index of the button of the group drawn at (x, y) that
// ev hits, or -1. Only a fresh press hits, and a hit consumes it: one press
// clicks one button. A miss leaves the press fresh, so a split row can try its
// groups in any order. The caller has called TrackPress for ev.
func (d *ModalDialog) buttonGroupHit(ev *tcell.EventMouse, x, y int, labels []string) int {
	if !d.fresh {
		return -1
	}
	mx, my := ev.Position()
	if my != y {
		return -1
	}
	col := x
	for i, label := range labels {
		w := core.DisplayWidth("[ " + label + " ]")
		if mx >= col && mx < col+w {
			d.fresh = false
			return i
		}
		col += w + 2
	}
	return -1
}

// ---------------------------------------------------------------------------
// Split button row: a left group and a right-aligned group
// ---------------------------------------------------------------------------

// ButtonLayout places a split button row's two groups (see SplitButtonLayout).
// Drawing and hit-testing share it, so clicks land where the buttons show.
type ButtonLayout struct {
	LeftX, LeftY   int // where the left group starts
	RightX, RightY int // where the right-aligned group starts; RightY is ButtonRowY

	// GapX, GapW are the free columns on the right group's row left of it: after
	// the left group when they share the row, from the left margin when it
	// dropped below. A dialog draws status there (a spinner, "Connecting...").
	GapX, GapW int
}

// SplitButtonLayout lays out a button row whose left group (e.g. a destructive
// button) sits at the left margin and whose right group is right-aligned as
// DrawButtons places it. When the two groups do not fit side by side — a
// dialog clamped onto a narrow terminal — the left group moves to the row
// below, which is blank by the button-row rule; they never overlap. A group
// wider than the box on its own is cut at the border by DrawBase's clip.
func (d *ModalDialog) SplitButtonLayout(left, right []string) ButtonLayout {
	marginX := d.InnerRect().X + 1
	l := ButtonLayout{
		LeftX: marginX, LeftY: d.ButtonRowY(),
		RightX: d.ButtonRowStartX(right), RightY: d.ButtonRowY(),
	}
	if len(left) == 0 {
		l.GapX = marginX
	} else if lw := d.ButtonRowWidth(left); marginX+lw+1 <= l.RightX {
		// One free column between the groups is enough to tell them apart;
		// the gap shrinks to that before the left group gives up its row.
		l.GapX = marginX + lw + 2
	} else {
		l.LeftY = d.ButtonRowY() + 1
		l.GapX = marginX
	}
	l.GapW = max(0, l.RightX-2-l.GapX)
	return l
}

// DrawSplitButtons draws a split button row laid out by SplitButtonLayout.
// activeIdx and disabled index the whole row left to right: the left group's
// buttons first, then the right group's.
func (d *ModalDialog) DrawSplitButtons(s tcell.Screen, left, right []string, activeIdx int, disabled []bool) {
	l := d.SplitButtonLayout(left, right)
	n := len(left)
	d.clearButtonRows(s)
	d.drawButtonGroup(s, l.LeftX, l.LeftY, left, activeIdx, disabled[:min(n, len(disabled))])
	d.drawButtonGroup(s, l.RightX, l.RightY, right, activeIdx-n, disabled[min(n, len(disabled)):])
}

// SplitButtonClicked is ButtonClicked for a row drawn by DrawSplitButtons. It
// answers an index into the whole row, numbered as DrawSplitButtons numbers it,
// or -1.
func (d *ModalDialog) SplitButtonClicked(ev *tcell.EventMouse, left, right []string) int {
	d.TrackPress(ev)
	l := d.SplitButtonLayout(left, right)
	if i := d.buttonGroupHit(ev, l.LeftX, l.LeftY, left); i >= 0 {
		return i
	}
	if i := d.buttonGroupHit(ev, l.RightX, l.RightY, right); i >= 0 {
		return i + len(left)
	}
	return -1
}
