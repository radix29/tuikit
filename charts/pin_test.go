package charts

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/theme"
)

// pinFixture is plotHitFixture's chart (plot at x 5..14, 4 buckets) seen
// through an unscrolled viewport, with bucket i named "t<i>".
func pinFixture() ([]PlotHit, *core.Viewport) {
	vp := &core.Viewport{View: core.Rect{X: 0, Y: 0, W: 30, H: 10}, ContentW: 30, ContentH: 10}
	return []PlotHit{plotHitFixture()}, vp
}

func pinTimeAt(idx int) string { return fmt.Sprintf("t%d", idx) }

// pinIndexOf resolves "t<i>" against a chart of n buckets, -1 past it.
func pinIndexOf(n int) func(string) int {
	return func(at string) int {
		var i int
		if _, err := fmt.Sscanf(at, "t%d", &i); err != nil || i >= n {
			return -1
		}
		return i
	}
}

// grow appends one bucket to every series, pushing the old ones a column left.
func grow(hit *PlotHit, v float64) {
	for i := range hit.Series {
		hit.Series[i].Values = append(slices.Clone(hit.Series[i].Values), v)
	}
}

func TestPinAtPinsTheClickedBucket(t *testing.T) {
	hits, vp := pinFixture()
	p := PinAt(hits, vp, 14, 3, pinTimeAt)
	if p == nil {
		t.Fatal("a click on the newest column pinned nothing")
	}
	if p.Chart != "Batches" || p.Time != "t3" || p.Col != 14 || p.Row != 3 {
		t.Errorf("pin = %q %q at (%d,%d), want Batches t3 at (14,3)", p.Chart, p.Time, p.Col, p.Row)
	}
	if len(p.Rows) != 2 || p.Rows[1].Value != FormatValue(40) || p.Rows[1].Color != colB {
		t.Errorf("rows = %+v, want one per series, the short one reading 40", p.Rows)
	}
	if older := PinAt(hits, vp, 13, 3, pinTimeAt); older == nil || older.Time != "t2" {
		t.Errorf("one column left pinned %+v, want t2", older)
	}
}

func TestPinAtMisses(t *testing.T) {
	hits, vp := pinFixture()
	for _, c := range []struct {
		name string
		x, y int
	}{
		{"outside the plot", 2, 3},
		{"a column predating the data", 5, 3},
		{"outside the view", 40, 3},
	} {
		if p := PinAt(hits, vp, c.x, c.y, pinTimeAt); p != nil {
			t.Errorf("%s pinned %+v, want nil", c.name, p)
		}
	}
}

// The click translates through the viewport: hits are canvas cells.
func TestPinAtTranslatesThroughTheViewport(t *testing.T) {
	hits, vp := pinFixture()
	vp.ContentW, vp.View.W = 40, 10
	vp.X = 6 // canvas 6 at screen 0
	p := PinAt(hits, vp, 8, 3, pinTimeAt)
	if p == nil || p.Col != 14 || p.Time != "t3" {
		t.Errorf("screen 8 (canvas 14) pinned %+v, want t3 at canvas 14", p)
	}
}

// A new sample pushes the pinned bucket left: the pin follows it and keeps
// quoting its own values.
func TestPinRefreshFollowsItsBucket(t *testing.T) {
	hits, vp := pinFixture()
	p := PinAt(hits, vp, 14, 3, pinTimeAt)
	want := p.Rows[0].Value

	grow(&hits[0], 99)
	if !p.Refresh(hits, vp, pinIndexOf(5), pinTimeAt) {
		t.Fatal("the pin closed; its bucket is still drawn")
	}
	if p.Time != "t3" || p.Col != 13 {
		t.Errorf("pin = %q at column %d, want t3 one column left (13)", p.Time, p.Col)
	}
	if p.Rows[0].Value != want {
		t.Errorf("pin reads %q, want its own bucket's %q", p.Rows[0].Value, want)
	}
}

func TestPinRefreshCloses(t *testing.T) {
	t.Run("chart gone", func(t *testing.T) {
		hits, vp := pinFixture()
		p := PinAt(hits, vp, 14, 3, pinTimeAt)
		if p.Refresh(nil, vp, pinIndexOf(4), pinTimeAt) {
			t.Error("the pin survived its chart")
		}
	})
	t.Run("bucket pruned", func(t *testing.T) {
		hits, vp := pinFixture()
		p := PinAt(hits, vp, 14, 3, pinTimeAt)
		if p.Refresh(hits, vp, func(string) int { return -1 }, pinTimeAt) {
			t.Error("the pin survived its bucket's pruning")
		}
	})
	t.Run("aged off the plot", func(t *testing.T) {
		hits, vp := pinFixture()
		hits[0].Series = []Series{{Label: "full", Color: colA, Values: make([]float64, 10)}}
		p := PinAt(hits, vp, 5, 3, pinTimeAt) // oldest drawn bucket, t0
		if p == nil || p.Time != "t0" {
			t.Fatalf("pin = %+v, want t0", p)
		}
		grow(&hits[0], 1)
		if p.Refresh(hits, vp, pinIndexOf(11), pinTimeAt) {
			t.Errorf("the pin survived its bucket leaving the plot, at column %d", p.Col)
		}
	})
	t.Run("scrolled out of view", func(t *testing.T) {
		hits, vp := pinFixture()
		p := PinAt(hits, vp, 14, 3, pinTimeAt)
		vp.View.Y, vp.Y, vp.ContentH = 0, 5, 20
		if p.Refresh(hits, vp, pinIndexOf(4), pinTimeAt) {
			t.Error("the pin survived its point leaving the view")
		}
	})
}

// A snapshot pin stays on its click column and re-captions with the newest
// sample.
func TestPinRefreshSnapshotFollowsTheNewestSample(t *testing.T) {
	hits, vp := pinFixture()
	hits[0].Snapshot = true
	hits[0].Series = []Series{{Label: "now", Color: colA, Values: []float64{7}}}
	p := PinAt(hits, vp, 8, 3, func(int) string { return "12:00:00" })
	if p == nil || !p.Snapshot || p.Col != 8 {
		t.Fatalf("pin = %+v, want a snapshot pin at column 8", p)
	}
	noIndex := func(string) int { t.Error("a snapshot pin looked its time up"); return -1 }
	if !p.Refresh(hits, vp, noIndex, func(int) string { return "12:00:02" }) {
		t.Fatal("the snapshot pin closed")
	}
	if p.Time != "12:00:02" || p.Col != 8 {
		t.Errorf("pin = %q at column %d, want 12:00:02 at 8", p.Time, p.Col)
	}
}

// Draw calls the time out on the axis and puts the box beside the point.
func TestPinDrawCalloutAndBox(t *testing.T) {
	hits, vp := pinFixture()
	hits[0].TimeRow = core.Rect{X: 5, Y: 6, W: 10, H: 1}
	canvas := core.NewBuffer(30, 10)
	core.DrawText(canvas, 5, 6, theme.StyleChartAxis(), "-0:20    0")
	s := core.NewBuffer(30, 10)
	canvas.Blit(s, canvas.Rect(), vp.View)

	p := PinAt(hits, vp, 13, 3, pinTimeAt) // t2
	if p == nil {
		t.Fatal("no pin to draw")
	}
	p.Draw(s, canvas, vp)
	if !strings.Contains(s.Row(6), "t2") {
		t.Errorf("axis row = %q, want the pinned time called out", s.Row(6))
	}
	var box bool
	for y := range 10 {
		box = box || strings.Contains(s.Row(y), "long")
	}
	if !box {
		t.Error("the readout wasn't drawn")
	}
}

// A box that doesn't fit the view isn't drawn at all.
func TestPinDrawNoRoom(t *testing.T) {
	hits, vp := pinFixture()
	p := PinAt(hits, vp, 14, 3, pinTimeAt)
	vp.View = core.Rect{X: 0, Y: 0, W: 30, H: 3}
	s := core.NewBuffer(30, 10)
	p.Draw(s, nil, vp)
	for y := range 10 {
		if strings.Contains(s.Row(y), "long") {
			t.Errorf("row %d = %q, want no box in a view too short for it", y, s.Row(y))
		}
	}
}
