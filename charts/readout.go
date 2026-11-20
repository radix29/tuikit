package charts

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// A chart readout is the box a click on a chart pins, naming what the colours
// under the pointer are worth: a caption (the moment, or the chart), then one
// row per series. This file owns the box, so every chart host's readout reads
// as the same thing; Pin (pin.go) owns its life — what identifies it, where it
// is each frame, when it closes — and the host decides what opens it.

// readoutPad is the space between a readout's frame and its text.
const readoutPad = 1

// ReadoutRow is one line of a readout: a series' label in that series' own
// colour, so a line in the box and a band in the chart are matched by eye,
// and its value right-aligned against it.
type ReadoutRow struct {
	Label string
	Value string
	Color tcell.Color
}

// ReadoutSize is the box caption and rows need, frame and padding included.
func ReadoutSize(caption string, rows []ReadoutRow) (w, h int) {
	w = core.DisplayWidth(caption)
	for _, row := range rows {
		if lw := core.DisplayWidth(row.Label) + core.DisplayWidth(row.Value) + 3; lw > w {
			w = lw
		}
	}
	return w + 2 + readoutPad*2, len(rows) + 3
}

// PlaceReadout positions a w×h box next to the pinned point (ax, ay),
// flipped to whichever side of it fits and clamped into view. A readout that
// hangs off the viewport is worse than no readout: the numbers it exists to
// show are the ones that get clipped.
//
// keepOut is a screen row the box must not cover — e.g. a time callout that
// names the moment every number in the box belongs to — or -1 for none. The
// box goes above the point instead when that side is clear of it.
func PlaceReadout(w, h, ax, ay int, view core.Rect, keepOut int) core.Rect {
	x := ax + 2
	if x+w > view.Right() {
		x = ax - w - 1
	}
	covers := func(top int) bool { return keepOut >= top && keepOut < top+h }
	y, above := ay+1, ay-h
	if y+h > view.Bottom() || (covers(y) && above >= view.Y && !covers(above)) {
		y = above
	}
	x = core.Clamp(x, view.X, max(view.Right()-w, view.X))
	y = core.Clamp(y, view.Y, max(view.Bottom()-h, view.Y))
	return core.Rect{X: x, Y: y, W: w, H: h}
}

// DrawReadout renders the readout at r: the caption, then one line per row.
// r comes from PlaceReadout at the size ReadoutSize asked for; anything that
// doesn't fit is clipped rather than wrapped.
func DrawReadout(s tcell.Screen, r core.Rect, caption string, rows []ReadoutRow) {
	body := theme.StyleTooltip()
	core.FillRect(s, r, ' ', body)
	core.DrawBox(s, r, theme.StyleTooltipBorder())

	x := r.X + 1 + readoutPad
	textW := r.W - 2 - readoutPad*2
	core.DrawTextClipped(s, x, r.Y+1, textW, body, caption)
	for i, row := range rows {
		y := r.Y + 2 + i
		if y >= r.Bottom()-1 {
			break
		}
		core.DrawTextClipped(s, x, y, textW, body.Foreground(row.Color), row.Label)
		core.DrawTextRight(s, x, y, textW, body, row.Value)
	}
}

// DrawAxisCallout writes text — the pinned bucket's moment — on a chart's
// time-axis row, centred under screen column x and kept within the row and
// vp's View: the row's other labels are times too, so a clipped callout would
// read as one of them. It returns the screen row used, or -1 when there is no
// room, for PlaceReadout's keepOut.
//
// row is the time row in canvas cells (PlotHit.TimeRow) and canvas is the
// buffer the chart was drawn on and blitted from through vp: the callout reads
// it to clear every label it overlaps whole, or a tail survives as a bogus
// number ("-0:20" + callout → "-0:211:34:44"). A host drawing straight to the
// screen passes a Viewport with a zero offset and the screen as canvas.
func DrawAxisCallout(s tcell.Screen, canvas *core.Buffer, vp *core.Viewport, row core.Rect, x int, text string) int {
	if row.W <= 0 || row.H <= 0 || text == "" {
		return -1
	}
	view := vp.View
	rowX, y := vp.ToScreen(row.X, row.Y)
	if y < view.Y || y >= view.Bottom() {
		return -1
	}
	w := core.DisplayWidth(text)
	left := max(rowX, view.X)
	right := min(rowX+row.W, view.Right())
	if right-left < w {
		return -1
	}
	x = core.Clamp(x-w/2, left, right-w)

	clearFrom, clearTo := axisLabelRun(canvas, vp, row, x, x+w)
	core.FillRect(s, core.Rect{X: clearFrom, Y: y, W: clearTo - clearFrom, H: 1}, ' ', theme.StyleChartAxis())
	core.DrawTextClipped(s, x, y, w, theme.StyleTooltip(), text)
	return y
}

// axisLabelRun widens the screen span [from, to) on row to cover every label it
// touches, stopping at blank columns and staying within the row and vp's View.
func axisLabelRun(canvas *core.Buffer, vp *core.Viewport, row core.Rect, from, to int) (int, int) {
	rowX, _ := vp.ToScreen(row.X, row.Y)
	lo := max(max(rowX, vp.View.X), 0)
	hi := min(rowX+row.W, vp.View.Right())

	blank := func(x int) bool {
		cx, _ := vp.ToCanvas(x, 0)
		str, _, _ := canvas.Get(cx, row.Y)
		return str == "" || str == " "
	}
	for from > lo && !blank(from-1) {
		from--
	}
	for to < hi && !blank(to) {
		to++
	}
	return from, to
}
