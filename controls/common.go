package controls

// Strings is the text this package's controls draw on their own: DataGrid's
// status and error column, its built-in context-menu entries and its value
// viewer, and ToolRow's overflow cell. A host may replace any of it (to
// translate it, say) on the UI goroutine before the first Draw; nothing
// dispatches on it. A grid reads Ready when created, Rows when a source
// arrives and the menu entries when the menu opens; a ToolRow reads More on
// every layout.
var Strings = struct {
	Ready, Error                      string
	Rows                              string // a grid's status once a source is set; %d is the row count
	Copy, CopyAll, CopyAllWithHeaders string
	ShowValue                         string
	ViewerHint, ViewerClose           string
	More                              string // ToolRow's cell for the cells it is too narrow to draw
}{
	Ready:              "Ready",
	Error:              "Error",
	Rows:               "%d rows",
	More:               "More ▾",
	Copy:               "Copy",
	CopyAll:            "Copy All",
	CopyAllWithHeaders: "Copy All with Headers",
	ShowValue:          "Show Value",
	ViewerHint:         "Esc or Close to dismiss — Shift+arrows to select, Ctrl+C to copy",
	ViewerClose:        "Close",
}

// ---------------------------------------------------------------------------
// Helpers shared across more than one file in this package
// ---------------------------------------------------------------------------

// hideToFit is the one overflow policy of Toolbar and ToolRow: it hides the
// entries of shown in order, one at a time, until need(shown) fits in avail,
// and reports whether it did. Only the order differs between the two —
// Toolbar's comes from DropRank, ToolRow's is right to left, so what a
// ToolRow hides is always the row's tail.
func hideToFit(shown []bool, order []int, need func(shown []bool) int, avail int) bool {
	for _, i := range order {
		if need(shown) <= avail {
			return true
		}
		shown[i] = false
	}
	return need(shown) <= avail
}
