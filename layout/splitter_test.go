package layout

import (
	"math"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/radix29/tuikit/tuikittest"
)

// TestSplitterDragOnlyStartsFromPressOnBar pins down that a resize begins
// only on a press that lands on the bar. A selection drag in the pane above
// that crosses the bar on its way down would otherwise grab it and resize
// the panes mid-selection — see the press field.
func TestSplitterDragOnlyStartsFromPressOnBar(t *testing.T) {
	sp := NewSplitter(SplitterHorizontal, 0.5)
	sp.SetBounds(0, 0, 80, 24)
	before := sp.Ratio()
	bar := sp.SplitPos()

	// Press well above the bar, then drag down across and past it.
	if sp.HandleMouse(tuikittest.Press(10, 2)) {
		t.Fatalf("press above the bar consumed by the splitter")
	}
	for _, y := range []int{bar - 2, bar, bar + 1, bar + 4} {
		if sp.HandleMouse(tuikittest.Press(10, y)) {
			t.Fatalf("motion at y=%d consumed by the splitter mid-gesture", y)
		}
	}
	sp.HandleMouse(tuikittest.Release(10, bar+4))

	if got := sp.Ratio(); got != before {
		t.Errorf("ratio = %v after a drag crossing the bar, want %v unchanged", got, before)
	}
}

// TestSplitterDragFromBarStillResizes is the other half: a gesture that
// really does begin on the bar must still resize, and must keep receiving
// events after the pointer leaves the bar's own row.
func TestSplitterDragFromBarStillResizes(t *testing.T) {
	sp := NewSplitter(SplitterHorizontal, 0.5)
	sp.SetBounds(0, 0, 80, 24)
	before := sp.Ratio()
	bar := sp.SplitPos()

	if !sp.HandleMouse(tuikittest.Press(10, bar)) {
		t.Fatalf("press on the bar not consumed")
	}
	if !sp.HandleMouse(tuikittest.Press(10, bar+3)) {
		t.Fatalf("motion off the bar not consumed while dragging")
	}
	if got := sp.Ratio(); got <= before {
		t.Fatalf("ratio = %v after dragging down, want greater than %v", got, before)
	}
	sp.HandleMouse(tuikittest.Release(10, bar+3))

	// The release cleared the latch, so the next press on the bar is a
	// fresh gesture rather than a continuation of the last one.
	moved := sp.Ratio()
	if !sp.HandleMouse(tuikittest.Press(10, sp.SplitPos())) {
		t.Fatalf("press on the bar after a completed drag not consumed")
	}
	sp.HandleMouse(tuikittest.Press(10, sp.SplitPos()-3))
	if got := sp.Ratio(); got >= moved {
		t.Errorf("ratio = %v after a second drag upward, want less than %v", got, moved)
	}
}

// A corrupt saved ratio must not reach the layout: core.Clamp passes NaN
// through, and a NaN ratio puts the bar anywhere.
func TestSplitterSetRatioIgnoresNaNAndInf(t *testing.T) {
	sp := NewSplitter(SplitterHorizontal, 0.5)
	sp.SetBounds(0, 0, 80, 24)
	before := sp.Ratio()
	for _, r := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		sp.SetRatio(r)
		if got := sp.Ratio(); got != before {
			t.Errorf("SetRatio(%v) left ratio %v, want %v unchanged", r, got, before)
		}
	}
}

// A resize to zero mid-drag must not divide by zero into a NaN ratio.
func TestSplitterDragSurvivesZeroSize(t *testing.T) {
	for _, sp := range []*Splitter{NewSplitter(SplitterHorizontal, 0.5), NewSplitter(SplitterVertical, 0.3)} {
		sp.SetBounds(0, 0, 80, 24)
		before := sp.Ratio()
		pos := sp.SplitPos()
		x, y := pos, 5
		if sp.dir == SplitterHorizontal {
			x, y = 10, pos
		}
		if !sp.HandleMouse(tuikittest.Press(x, y)) {
			t.Fatalf("dir %v: press on the bar not consumed", sp.dir)
		}
		sp.SetBounds(0, 0, 0, 0)
		sp.HandleMouse(tuikittest.Press(x, y)) // 0/0 before the fix
		if got := sp.Ratio(); math.IsNaN(got) || got != before {
			t.Errorf("dir %v: ratio = %v after a drag at zero size, want %v", sp.dir, got, before)
		}
		sp.HandleMouse(tuikittest.Release(x, y))
	}
}

// TestSplitterLimits pins C11 of the 2026-11-12 review: the limits are the
// host's, SetLimits re-clamps the ratio, and nonsense limits are ignored.
func TestSplitterLimits(t *testing.T) {
	sp := NewSplitter(SplitterVertical, 0.95)
	if got := sp.Ratio(); got != defaultMaxRatio {
		t.Fatalf("ratio = %v, want the default max %v", got, defaultMaxRatio)
	}
	sp.SetLimits(0.2, 0.6)
	if got := sp.Ratio(); got != 0.6 {
		t.Fatalf("ratio after SetLimits(0.2, 0.6) = %v, want 0.6", got)
	}
	sp.SetRatio(0.1)
	if got := sp.Ratio(); got != 0.2 {
		t.Fatalf("SetRatio(0.1) = %v, want the new min 0.2", got)
	}
	for _, lim := range [][2]float64{{0.7, 0.3}, {-0.1, 0.5}, {0.5, 1.1}, {math.NaN(), 0.5}} {
		sp.SetLimits(lim[0], lim[1])
		if sp.minRatio != 0.2 || sp.maxRatio != 0.6 {
			t.Errorf("SetLimits(%v, %v) changed the limits to %v–%v", lim[0], lim[1], sp.minRatio, sp.maxRatio)
		}
	}
}

// TestSplitterDragKeepsBarUnderPointer pins bug 3 of the 2026-11-19 round-2
// review: a drag stored ratioBase+delta/size, and SplitPos's floor landed one
// cell short whenever float error left size*ratio just below the integer (10
// of the columns from 0.3 on 80 wide). Every column the limits allow must put
// the bar exactly under the pointer, in both directions and both orientations.
func TestSplitterDragKeepsBarUnderPointer(t *testing.T) {
	for _, dir := range []SplitterDir{SplitterVertical, SplitterHorizontal} {
		for size := 10; size <= 300; size++ {
			for _, start := range []float64{0.25, 0.3, 0.5, 0.7} {
				for _, reverse := range []bool{false, true} {
					sp := NewSplitter(dir, start)
					const origin = 3
					at := func(p int) (int, int) { return p, 5 }
					if dir == SplitterHorizontal {
						sp.SetBounds(0, origin, 40, size)
						at = func(p int) (int, int) { return 5, p }
					} else {
						sp.SetBounds(origin, 0, size, 40)
					}
					if !sp.HandleMouse(tuikittest.Press(at(sp.SplitPos()))) {
						t.Fatalf("dir %v size %d start %v: press on the bar not consumed", dir, size, start)
					}
					for i := range size {
						p := origin + i
						if reverse {
							p = origin + size - 1 - i
						}
						sp.HandleMouse(tuikittest.Press(at(p)))
						r := (float64(i) + 0.5) / float64(size)
						if reverse {
							r = (float64(size-1-i) + 0.5) / float64(size)
						}
						if r < sp.minRatio || r > sp.maxRatio {
							continue
						}
						if got := sp.SplitPos(); got != p {
							t.Fatalf("dir %v size %d start %v reverse %v: pointer at %d, bar at %d", dir, size, start, reverse, p, got)
						}
					}
				}
			}
		}
	}
}

// A fixed 0.05 ratio step is half a cell on a 10-wide split; every press must
// move the bar at least one cell.
func TestSplitterKeyStepMovesAtLeastOneCell(t *testing.T) {
	sp := NewSplitter(SplitterVertical, 0.3)
	sp.SetBounds(0, 0, 10, 5)
	for range 4 {
		before := sp.SplitPos()
		if !sp.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModCtrl)) {
			t.Fatalf("Ctrl+Right not consumed")
		}
		if got := sp.SplitPos(); got != before+1 {
			t.Fatalf("Ctrl+Right moved the bar %d -> %d, want one cell", before, got)
		}
	}
	before := sp.SplitPos()
	sp.HandleKey(tuikittest.Key(tcell.KeyLeft, tcell.ModCtrl))
	if got := sp.SplitPos(); got != before-1 {
		t.Fatalf("Ctrl+Left moved the bar %d -> %d, want one cell", before, got)
	}

	// On 80 wide the step is round(0.05*80) = 4 cells.
	wide := NewSplitter(SplitterHorizontal, 0.5)
	wide.SetBounds(0, 0, 40, 80)
	before = wide.SplitPos()
	wide.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModCtrl))
	if got := wide.SplitPos(); got != before+4 {
		t.Fatalf("Ctrl+Down on 80 high moved the bar %d -> %d, want 4 cells", before, got)
	}
	if wide.HandleKey(tuikittest.Key(tcell.KeyRight, tcell.ModCtrl)) {
		t.Errorf("Ctrl+Right consumed by a horizontal splitter")
	}
	if wide.HandleKey(tuikittest.Key(tcell.KeyDown, tcell.ModCtrl, tcell.ModShift)) {
		t.Errorf("Ctrl+Shift+Down consumed by the splitter")
	}
}

// SetLimits allows a max of 1, which put the bar one past the rect and gave
// SecondRect a width of -1.
func TestSplitterBarStaysInsideRect(t *testing.T) {
	for _, dir := range []SplitterDir{SplitterVertical, SplitterHorizontal} {
		sp := NewSplitter(dir, 0.5)
		sp.SetBounds(2, 3, 20, 10)
		sp.SetLimits(0, 1)
		sp.SetRatio(1)
		origin, size := sp.axis()
		if got := sp.SplitPos(); got != origin+size-1 {
			t.Errorf("dir %v: ratio 1 puts the bar at %d, want the last cell %d", dir, got, origin+size-1)
		}
		if r := sp.SecondRect(); r.W < 0 || r.H < 0 {
			t.Errorf("dir %v: SecondRect = %+v at ratio 1", dir, r)
		}
		sp.SetRatio(0)
		if got := sp.SplitPos(); got != origin {
			t.Errorf("dir %v: ratio 0 puts the bar at %d, want %d", dir, got, origin)
		}
		if r := sp.FirstRect(); r.W < 0 || r.H < 0 {
			t.Errorf("dir %v: FirstRect = %+v at ratio 0", dir, r)
		}
	}
}

// TestStackSplitterPreset pins the stacked-panes preset: a horizontal bar at
// half the rect, the default limits, and the label drawn on the bar.
func TestStackSplitterPreset(t *testing.T) {
	label := "─── Results ─── " + ResizeHint
	sp := NewStackSplitter(label)
	if sp.dir != SplitterHorizontal || sp.Ratio() != 0.5 {
		t.Fatalf("dir, ratio = %v, %v; want horizontal, 0.5", sp.dir, sp.Ratio())
	}
	if sp.minRatio != defaultMinRatio || sp.maxRatio != defaultMaxRatio {
		t.Fatalf("limits = %v–%v, want the defaults %v–%v", sp.minRatio, sp.maxRatio, defaultMinRatio, defaultMaxRatio)
	}
	sp.SetRatio(0.95)
	if got := sp.Ratio(); got != defaultMaxRatio {
		t.Fatalf("SetRatio(0.95) = %v, want clamped to %v", got, defaultMaxRatio)
	}
	sp.SetRatio(0.5)

	sp.SetBounds(0, 0, 60, 20)
	s := tuikittest.NewScreen(60, 20)
	sp.Draw(s)
	if x, y, ok := s.Find(label); !ok || y != sp.SplitPos() || x != 2 {
		t.Errorf("label at (%d,%d) found=%v, want (2,%d) on the bar", x, y, ok, sp.SplitPos())
	}
}
