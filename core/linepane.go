package core

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/theme"
)

// LinePane is the scroll state of a read-only pane of text lines (a details
// pane under a grid, a properties strip): lines drawn from offset Scroll with a
// one-column inset each side, clipped, and a dim hint in the bottom-right
// corner while lines are hidden below; DrawEmpty is the pane with a dim
// placeholder instead (nothing selected).
//
// Like Viewport it is a value the host embeds, not a control: the host builds
// the lines (and caches them, keyed on its own item), routes keys, wheel
// (LineWheel) and focus, and passes the same line count to Draw and ScrollBy so the last line
// is always reachable and never overshot.
type LinePane struct {
	Scroll int    // the first line drawn
	More   string // drawn dim bottom-right while lines are hidden below; "" for none
}

// MaxScroll is the largest Scroll that keeps n lines filling h rows: zero
// when they fit.
func (p *LinePane) MaxScroll(n, h int) int { return max(n-h, 0) }

// ScrollTo moves Scroll to line, clamped for n lines in h rows, and reports
// whether it moved.
func (p *LinePane) ScrollTo(line, n, h int) bool {
	line = Clamp(line, 0, p.MaxScroll(n, h))
	if line == p.Scroll {
		return false
	}
	p.Scroll = line
	return true
}

// ScrollBy is ScrollTo relative to the current Scroll.
func (p *LinePane) ScrollBy(delta, n, h int) bool {
	return p.ScrollTo(p.Scroll+delta, n, h)
}

// LineWheel is the scroll a wheel tick over a LinePane's rect r asks for:
// delta is -1 for WheelUp, +1 for WheelDown, and ok is false for any other
// event or a pointer outside r. The host passes delta to its ScrollBy and
// claims the event whenever ok, so a tick at the top or bottom is swallowed
// rather than falling through to whatever is under the pane.
//
// Shift+WheelUp/Down scroll vertically too, unlike WheelDelta (which turns
// them sideways for Viewport): a LinePane has no horizontal axis, so the
// desktop convention would leave Shift+wheel dead over it. WheelLeft/Right
// are not ticks for it (ok is false) and fall through.
//
// A function, not a ScrollBy wrapper, because hosts build the line count
// from the selected item (wrapping it to the pane's width) and should only
// pay for that on a tick that scrolls, and because r is the hit area, which
// may be taller than the rows drawn (a pane with a heading row).
func LineWheel(ev *tcell.EventMouse, r Rect) (delta int, ok bool) {
	var d int
	switch ev.Buttons() {
	case tcell.WheelUp:
		d = -1
	case tcell.WheelDown:
		d = 1
	default:
		return 0, false
	}
	if !r.Contains(ev.Position()) {
		return 0, false
	}
	return d, true
}

// CanUp reports whether lines are hidden above.
func (p *LinePane) CanUp() bool { return p.Scroll > 0 }

// CanDown reports whether some of n lines are hidden below h rows.
func (p *LinePane) CanDown(n, h int) bool { return n-p.Scroll-h > 0 }

// Draw fills r with base and draws lines into it from Scroll, each in
// style(i) for line index i (base when style is nil). Scroll is clamped
// first, so a pane grown taller, or text rewrapped shorter, shows its last
// line at the bottom rather than blank rows under it.
func (p *LinePane) Draw(s tcell.Screen, r Rect, base tcell.Style, lines []string, style func(i int) tcell.Style) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	FillRect(s, r, ' ', base)
	p.ScrollTo(p.Scroll, len(lines), r.H)
	for row := range min(r.H, len(lines)-p.Scroll) {
		i := p.Scroll + row
		st := base
		if style != nil {
			st = style(i)
		}
		DrawTextClipped(s, r.X+1, r.Y+row, r.W-2, st, lines[i])
	}
	if p.More != "" && p.CanDown(len(lines), r.H) {
		DrawTextRight(s, r.X, r.Y+r.H-1, r.W-1, base.Foreground(theme.Active().TextDim), p.More)
	}
}

// DrawEmpty fills r with base and draws text dim on its first row, inset like
// Draw's lines: the pane while nothing is selected. A separate call, not an
// empty lines slice, so a pane too narrow to wrap its item into lines stays
// blank instead of claiming nothing is selected.
func (p *LinePane) DrawEmpty(s tcell.Screen, r Rect, base tcell.Style, text string) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	FillRect(s, r, ' ', base)
	DrawTextClipped(s, r.X+1, r.Y, r.W-2, base.Foreground(theme.Active().TextDim), text)
}
