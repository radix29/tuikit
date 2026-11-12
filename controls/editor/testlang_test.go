package editor

import (
	"github.com/radix29/tuikit/theme"
)

// ---------------------------------------------------------------------------
// A toy language for tests and benchmarks that need a highlighter whose state
// crosses lines. tuikit knows no real language with that shape but XML; this
// one is shaped like the host's T-SQL (nested /* */ comments, "--" line
// comments, '...' strings that end with their line), lexed just well enough to
// exercise LineStateAt and prefixStates the way a real highlighter does.
// ---------------------------------------------------------------------------

// toyStep is the one-line transition: given the block-comment depth a line
// begins in, the depth it leaves open.
func toyStep(line []rune, depth int) int {
	for i := 0; i < len(line); i++ {
		switch {
		case depth > 0 && toyAt(line, i, '*', '/'):
			depth--
			i++
		case toyAt(line, i, '/', '*'):
			depth++
			i++
		case depth == 0 && toyAt(line, i, '-', '-'):
			return 0
		case depth == 0 && line[i] == '\'':
			for i++; i < len(line) && line[i] != '\''; i++ {
			}
		}
	}
	return depth
}

func toyAt(line []rune, i int, a, b rune) bool {
	return i+1 < len(line) && line[i] == a && line[i+1] == b
}

// toyKey owns the toy language's LineStateAt cache.
type toyKey struct{}

// toyLineState is the depth line idx of doc begins in, from doc's cache.
func toyLineState(doc *Document, idx int) int {
	return LineStateAt(doc, toyKey{}, idx, 0, toyStep)
}

// toyFullReplay is toyLineState with no cache: toyStep folded over
// lines[0:idx].
func toyFullReplay(lines [][]rune, idx int) int {
	depth := 0
	for i := range idx {
		depth = toyStep(lines[i], depth)
	}
	return depth
}

// toyHighlighter colours a line that begins inside a comment as one comment run,
// and every other line as one keyword run: enough to make Draw pay for the
// line-state lookup the way a real cross-line highlighter does.
func toyHighlighter(p *theme.Palette) Highlighter {
	return func(doc *Document, idx int) []ColorRun {
		n := len(doc.Line(idx))
		if toyLineState(doc, idx) > 0 {
			return []ColorRun{{0, n, theme.On(p.EditorBg, p.EditorComment)}}
		}
		return []ColorRun{{0, n, theme.On(p.EditorBg, p.EditorKeyword)}}
	}
}
