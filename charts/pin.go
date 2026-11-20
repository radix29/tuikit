package charts

import (
	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/core"
)

// Pin is a readout pinned by a click on a chart: every series of that chart at
// the clicked bucket. readout.go owns the box; Pin owns the pin's life — what
// it is pinned to, where that is on each frame, and when it must close.
//
// It pins the bucket (chart title and bucket time), not the screen spot. A
// history pushes its buckets left as samples arrive, so a pin that kept its
// column would quote whatever sample drifted under it; Refresh re-derives the
// column from the time on every frame and drops the pin once that bucket is
// pruned or ages past the plot's left edge.
//
// Positions are canvas cells (the coordinates the host drew the charts and
// recorded its PlotHits in), translated through a core.Viewport at the edges,
// as DrawAxisCallout does. A host drawing straight to the screen passes a
// Viewport with a zero offset and View set to the area it drew in.
//
// The host keeps a *Pin (nil when nothing is pinned) and decides what else
// closes it: the next click, a pan, a tab switch.
type Pin struct {
	// Chart is the PlotHit.Title of the chart the pin lives on.
	Chart string
	// Time is the pinned bucket's moment: its identity and the readout's
	// caption.
	Time string
	Rows []ReadoutRow

	// Snapshot marks a pin on a current-sample chart (PlotHit.Snapshot): one
	// bucket, no time axis, so nothing drifts and the caption follows the
	// newest sample.
	Snapshot bool

	// Col and Row are the pinned point: the bucket's column and the clicked
	// row within the plot.
	Col, Row int

	// Plot and TimeRow are the pinned chart's rects as of the last Refresh.
	Plot, TimeRow core.Rect
}

// PinAt pins the bucket under a click at screen (x, y), or returns nil when
// the click is outside vp's View, misses every plot, or hits a column with no
// sample. hits are the last draw's, in canvas cells; timeAt names bucket idx
// of a chart (a snapshot chart asks for 0).
func PinAt(hits []PlotHit, vp *core.Viewport, x, y int, timeAt func(idx int) string) *Pin {
	if !vp.View.Contains(x, y) {
		return nil
	}
	cx, cy := vp.ToCanvas(x, y)
	for _, hit := range hits {
		if !hit.Plot.Contains(cx, cy) {
			continue
		}
		idx := hit.Bucket(cx)
		if idx < 0 {
			return nil
		}
		p := &Pin{Chart: hit.Title, Snapshot: hit.Snapshot, Col: cx, Row: cy}
		p.read(hit, idx, timeAt)
		return p
	}
	return nil
}

// Refresh re-resolves the pin against this frame's hits, moving it onto its
// bucket's current column and re-reading its values. It reports false when the
// pin must close: its chart is gone, indexOf no longer finds its time (-1:
// pruned), the bucket has aged off the plot, or the point is outside vp's
// View.
//
// Call it after the draw that rebuilt hits and before Draw: earlier reads last
// frame's geometry. indexOf maps a time back to its bucket index now (indexes
// shift with every sample; search newest first so samples sharing a label
// resolve consistently); a snapshot pin skips it.
func (p *Pin) Refresh(hits []PlotHit, vp *core.Viewport, indexOf func(time string) int, timeAt func(idx int) string) bool {
	hit, ok := hitByTitle(hits, p.Chart)
	if !ok {
		return false
	}
	idx := 0
	if !p.Snapshot {
		idx = indexOf(p.Time)
	}
	if idx < 0 {
		return false
	}
	p.read(hit, idx, timeAt)

	x, y := vp.ToScreen(p.Col, p.Row)
	return p.Col >= 0 && hit.Plot.Contains(p.Col, p.Row) && vp.View.Contains(x, y)
}

// read fills the pin from bucket idx of hit: its time, current column, the
// chart's rects, and one row per series in the series' colour.
func (p *Pin) read(hit PlotHit, idx int, timeAt func(int) string) {
	p.Time = timeAt(idx)
	p.Plot, p.TimeRow = hit.Plot, hit.TimeRow
	if !p.Snapshot {
		p.Col = hit.Column(idx)
	}
	p.Rows = p.Rows[:0]
	for _, ser := range hit.Series {
		p.Rows = append(p.Rows, ReadoutRow{
			Label: ser.Label,
			Value: FormatValue(hit.Value(ser, idx)),
			Color: ser.Color,
		})
	}
}

// hitByTitle is the hit recorded for one chart.
func hitByTitle(hits []PlotHit, title string) (PlotHit, bool) {
	for _, hit := range hits {
		if hit.Title == title {
			return hit, true
		}
	}
	return PlotHit{}, false
}

// Draw renders the pin over the blitted canvas: its time called out on the
// chart's time axis (DrawAxisCallout), then the readout beside the point,
// placed clear of that callout. canvas is the buffer the charts were drawn on,
// or nil to skip the callout.
//
// A box larger than vp's View is not drawn at all: clipped, it would hide the
// very numbers it exists to show.
func (p *Pin) Draw(s tcell.Screen, canvas *core.Buffer, vp *core.Viewport) {
	view := vp.View
	if view.W <= 0 || view.H <= 0 {
		return
	}
	callout := p.drawCallout(s, canvas, vp)

	ax, ay := vp.ToScreen(p.Col, p.Row)
	w, h := ReadoutSize(p.Time, p.Rows)
	if w > view.W || h > view.H {
		return
	}
	DrawReadout(s, PlaceReadout(w, h, ax, ay, view, callout), p.Time, p.Rows)
}

// drawCallout names the pin's moment on its chart's time axis and returns the
// screen row used, or -1, for the box to keep clear of.
func (p *Pin) drawCallout(s tcell.Screen, canvas *core.Buffer, vp *core.Viewport) int {
	if canvas == nil || p.Plot.W <= 0 {
		return -1
	}
	x, _ := vp.ToScreen(p.Col, 0)
	if x < vp.View.X || x >= vp.View.Right() {
		return -1
	}
	return DrawAxisCallout(s, canvas, vp, p.TimeRow, x, p.Time)
}
