package dialogs

import (
	"fmt"
	"slices"
	"strings"

	"github.com/radix29/tuikit/core"
	"github.com/radix29/tuikit/widgets"
)

// FileDialogMode selects Open (pick an existing file) or Save (pick or
// confirm a destination path) behaviour for FileDialog.
type FileDialogMode int

const (
	FileDialogOpen FileDialogMode = iota
	FileDialogSave
)

// Geometry: fileDialogW/H size the dialog, fileListRows is how many entries are
// visible at once, and fileSizeColW/fileModColW are the right-hand columns'
// widths (Name takes what is left).
const (
	fileDialogW  = 76
	fileDialogH  = 24
	fileListRows = 12
	fileSizeColW = 10
	fileModColW  = 17
)

// Tab-order focus stops.
const (
	ffPath = iota
	ffList
	ffName
	ffButtons
)

// FileDialog is a generic Open/Save file picker: a persistent path bar, a
// scrollable Name/Size/Modified listing and a filename field, with shell-style
// Tab completion on both text fields. It reaches the filesystem only through a
// FileSystem (no os/filepath calls, no SQL Server knowledge), so one instance
// serves local (ShowOpen/ShowSave) and remote (ShowOpenOn/ShowSaveOn) hosts.
type FileDialog struct {
	ModalDialog

	mode FileDialogMode

	// fs is the filesystem being browsed. Every Show* entry point sets it, so one
	// caller's remote filesystem can't leak into the next local browse.
	fs FileSystem

	dir     string
	entries []FileEntry
	listErr string
	// busy, when set, replaces the listing with a one-line message while a
	// FileSystem call is in flight — see showBusy.
	busy      string
	sel       int
	scroll    int
	typeahead string

	// listPress separates a fresh Button1 press on the list from a resent hold
	// (as TreeView): otherwise one click on an already-selected row could call
	// activateSelected repeatedly.
	listPress core.PressLatch
	// clicks pairs fresh presses on one row into the double-click that opens it
	// (as ListBox); a single click only selects, whichever row was selected.
	clicks core.ClickTracker[int]

	// drag is the text-selection gesture a click in pathField or nameField starts;
	// see FieldGesture for the ordering its three calls need.
	drag FieldGesture

	pathField *widgets.InputField
	nameField *widgets.InputField

	focus    int
	btnFocus int

	// OnChoose fires once the user confirms a file (Open) or destination path
	// (Save) — in Save mode, after any OnConfirmOverwrite round-trip.
	OnChoose func(path string)
	// OnCancel fires on Escape or the Cancel button. Optional.
	OnCancel func()
	// OnConfirmOverwrite, if set, is called instead of OnChoose (Save mode only)
	// when the chosen path already exists. The host asks however it likes and
	// calls proceed() to continue, or does nothing to leave the dialog open.
	OnConfirmOverwrite func(path string, proceed func())
}

// NewFileDialog creates the dialog.
func NewFileDialog() *FileDialog {
	d := &FileDialog{}
	d.InitModal(Strings.OpenFileTitle, fileDialogW, fileDialogH)
	// Both labels padded to the wider, so the two fields line up whatever the
	// host's Strings say.
	labelW := max(core.DisplayWidth(Strings.PathLabel), core.DisplayWidth(Strings.FileLabel))
	contentW := fileDialogW - 4     // InnerRect (-2) minus this dialog's own 1-col margin on each side
	fieldW := contentW - labelW - 3 // label gap + '[' + ']'
	d.pathField = widgets.NewInputField(core.PadRight(Strings.PathLabel, labelW), fieldW, false)
	d.nameField = widgets.NewInputField(core.PadRight(Strings.FileLabel, labelW), fieldW, false)
	return d
}

// ShowOpen configures the dialog to pick an existing file for reading and
// displays it. startPath seeds the initial directory and, if it names a file,
// the initial selection; "" means the working directory.
func (d *FileDialog) ShowOpen(title, startPath string, onChoose func(path string)) {
	d.ShowOpenOn(LocalFileSystem{}, title, startPath, onChoose)
}

// ShowOpenOn is ShowOpen against fs instead of the local machine.
func (d *FileDialog) ShowOpenOn(fs FileSystem, title, startPath string, onChoose func(path string)) {
	d.mode = FileDialogOpen
	d.fs = fs
	d.SetTitle(title)
	d.OnChoose = onChoose
	d.start(startPath, ffList)
}

// ShowSave configures the dialog to pick or confirm a destination path and
// displays it.
func (d *FileDialog) ShowSave(title, startPath string, onChoose func(path string)) {
	d.ShowSaveOn(LocalFileSystem{}, title, startPath, onChoose)
}

// ShowSaveOn is ShowSave against fs instead of the local machine.
func (d *FileDialog) ShowSaveOn(fs FileSystem, title, startPath string, onChoose func(path string)) {
	d.mode = FileDialogSave
	d.fs = fs
	d.SetTitle(title)
	d.OnChoose = onChoose
	d.start(startPath, ffName)
}

// FileSystem returns the filesystem the dialog is browsing — never nil once a
// Show* has run, the local machine before that.
func (d *FileDialog) FileSystem() FileSystem {
	if d.fs == nil {
		d.fs = LocalFileSystem{}
	}
	return d.fs
}

// start resets per-session state, loads startPath's directory, and shows
// the dialog with initialFocus focused.
func (d *FileDialog) start(startPath string, initialFocus int) {
	dir, name := d.splitStartPath(startPath)
	d.nameField.SetValue(name)
	d.entries, d.listErr = nil, ""
	d.btnFocus = 0
	// A latch must not survive into the next showing: a dialog dismissed mid-drag
	// would reopen still routing every click to that field.
	d.drag.Clear()
	d.setFocus(initialFocus)
	// Shown before the first listing: it is a FileSystem call like any other (the
	// slowest of the session on a remote one) and showBusy can only paint a visible
	// dialog.
	d.ModalDialog.Show()
	d.loadDir(dir)
	if name != "" {
		d.selectByName(name)
	}
}

// showBusy paints the dialog with msg in place of the listing and flushes it,
// ahead of a FileSystem call that may block. FileSystem is synchronous, so a
// remote call stops the TUI with the previous directory on screen, looking like
// a hang. This is the one place tuikit paints outside the app's draw cycle, so
// only a BlockingFileSystem gets a repaint (local browsing would flicker).
func (d *FileDialog) showBusy(msg string) {
	if d.screen == nil || !d.Visible() {
		return
	}
	if b, ok := d.FileSystem().(BlockingFileSystem); !ok || !b.Blocking() {
		return
	}
	d.busy = msg
	// Through a ClipScreen, as Stack.Draw does: on a plain screen a clamped
	// dialog's content is not clipped and spills across the UI for this frame.
	d.Draw(core.NewClipScreen(d.screen))
	d.screen.Show()
}

// entryFor returns the listing row for path when it names something in the
// directory already on screen (where a typed path usually points), sparing a
// FileSystem.Exists network round trip per keystroke.
func (d *FileDialog) entryFor(path string) (FileEntry, bool) {
	fs := d.FileSystem()
	dir, name := fs.Split(path)
	if name == "" || name == ".." || fs.Clean(dir) != d.dir {
		return FileEntry{}, false
	}
	for _, e := range d.entries {
		if e.Name == name {
			return e, true
		}
	}
	return FileEntry{}, false
}

// splitStartPath separates a caller-supplied initial path into a directory to
// open and a filename to preselect. "", a bare filename and a full path are all
// valid.
func (d *FileDialog) splitStartPath(startPath string) (dir, name string) {
	fs := d.FileSystem()
	if startPath == "" {
		return fs.Default(), ""
	}
	if dirPart, base := fs.Split(startPath); dirPart != "" {
		return dirPart, base
	}
	return fs.Default(), startPath
}

// loadDir lists dir's contents into d.entries (directories first, then files,
// both case-insensitively alphabetical), prefixed with ".." unless dir is the
// root. Resets selection and scroll and updates the path field.
func (d *FileDialog) loadDir(dir string) {
	fs := d.FileSystem()
	sep := fs.Separator()
	clean := fs.Clean(dir)
	d.dir = clean
	d.pathField.SetValue(strings.TrimSuffix(clean, sep) + sep)
	d.sel, d.scroll = 0, 0
	// A press on the old listing's row 2 and one on the new listing's are not a
	// double-click: without this, a quick double-click on a folder pairs its
	// second press with the first row of the folder it opened.
	d.clicks.Reset()

	d.showBusy(fmt.Sprintf(Strings.Listing, clean))
	infos, err := fs.List(clean)
	d.busy = ""
	if err != nil {
		d.entries = nil
		d.listErr = err.Error()
		return
	}
	d.listErr = ""

	var dirs, files []FileEntry
	for _, e := range infos {
		if e.Name == "." || e.Name == ".." {
			continue // the ".." row below is the dialog's own
		}
		if e.IsDir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	byLowerName := func(a, b FileEntry) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	slices.SortStableFunc(dirs, byLowerName)
	slices.SortStableFunc(files, byLowerName)

	entries := make([]FileEntry, 0, len(dirs)+len(files)+1)
	if parent := fs.Parent(clean); parent != clean {
		entries = append(entries, FileEntry{Name: "..", IsDir: true})
	}
	entries = append(entries, dirs...)
	entries = append(entries, files...)
	d.entries = entries
}

// selectByName moves the list selection to the entry named name, if any.
func (d *FileDialog) selectByName(name string) {
	for i, e := range d.entries {
		if e.Name == name {
			d.sel = i
			d.ensureVisible()
			return
		}
	}
}

// FocusedField returns the text field with keyboard focus (path bar or filename
// field), or nil when the list or button row is focused. Exported so a host's
// clipboard plumbing can resolve its target without FileDialog knowing about
// clipboards.
func (d *FileDialog) FocusedField() *widgets.InputField {
	switch d.focus {
	case ffPath:
		return d.pathField
	case ffName:
		return d.nameField
	}
	return nil
}

func (d *FileDialog) setFocus(f int) {
	d.focus = f
	d.pathField.Focus(f == ffPath)
	d.nameField.Focus(f == ffName)
	d.typeahead = ""
}

func (d *FileDialog) buttonLabels() []string {
	label := Strings.Open
	if d.mode == FileDialogSave {
		label = Strings.Save
	}
	return []string{label, Strings.Cancel}
}

// listRect returns the on-screen rectangle of the Name/Size/Modified list,
// shared by Draw and HandleMouse so hit-testing matches what was drawn.
func (d *FileDialog) listRect() core.Rect {
	inner := d.InnerRect()
	return core.Rect{X: inner.X + 1, Y: inner.Y + 4, W: inner.W - 2, H: fileListRows}
}

// nameColWidth returns the Name column's width for a list area contentW wide —
// what is left after the Size and Modified columns and their gaps.
func nameColWidth(contentW int) int {
	return contentW - fileSizeColW - fileModColW - 2
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

// confirmFocused runs Enter's action for whichever control has focus.
func (d *FileDialog) confirmFocused() {
	switch d.focus {
	case ffPath:
		d.navigateTyped()
	case ffList:
		d.activateSelected()
	case ffName:
		d.confirmChoice()
	case ffButtons:
		d.activateButton()
	}
}

// navigateTyped runs Enter on the path field: descends into the typed path if
// it is a directory, selects it and focuses the name field if it is an existing
// file, or tries to list it anyway and surfaces the error.
func (d *FileDialog) navigateTyped() {
	fs := d.FileSystem()
	typed := strings.TrimSpace(d.pathField.Value())
	target := d.dir
	if typed != "" {
		target = typed
		if !fs.IsAbs(target) {
			target = fs.Join(d.dir, target)
		}
		// A trailing ".." goes through Parent, as the ".." row does: Clean alone
		// stops at a UNC share root, where the row goes to the drive list.
		if base, ok := trimDotDot(target); ok {
			target = fs.Parent(base)
		}
	}
	isFile := false
	if e, ok := d.entryFor(target); ok {
		isFile = !e.IsDir
	} else {
		// A failed probe falls through to loadDir, which surfaces the failure in
		// listErr rather than swallowing it here.
		exists, isDir, _ := fs.Exists(target)
		isFile = exists && !isDir
	}
	if isFile {
		dir, name := fs.Split(target)
		d.loadDir(dir)
		d.nameField.SetValue(name)
		d.selectByName(name)
		d.setFocus(ffName)
		return
	}
	d.loadDir(target)
	d.setFocus(ffList)
}

// trimDotDot reports whether path ends in a ".." element and returns the path
// before it, or "" for a bare "..".
func trimDotDot(path string) (string, bool) {
	if path == ".." {
		return "", true
	}
	for _, sep := range []string{`\`, "/"} {
		if base, ok := strings.CutSuffix(path, sep+".."); ok {
			return base, true
		}
	}
	return "", false
}

// activateSelected runs Enter, or a double-click, on the list: descends into
// the selected directory, or confirms the selected file as the chosen path.
func (d *FileDialog) activateSelected() {
	if d.sel < 0 || d.sel >= len(d.entries) {
		return
	}
	fs := d.FileSystem()
	e := d.entries[d.sel]
	if e.IsDir {
		target := fs.Join(d.dir, e.Name)
		if e.Name == ".." {
			target = fs.Parent(d.dir)
		}
		d.loadDir(target)
		return
	}
	d.nameField.SetValue(e.Name)
	d.confirmChoice()
}

// syncNameFromSelection copies the selected entry's name into the name field
// unless it is a directory, so browsing folders leaves a typed filename
// alone.
func (d *FileDialog) syncNameFromSelection() {
	if d.sel < 0 || d.sel >= len(d.entries) {
		return
	}
	if e := d.entries[d.sel]; !e.IsDir {
		d.nameField.SetValue(e.Name)
	}
}

// confirmChoice builds the full path from the name field and current
// directory and runs finish on it.
func (d *FileDialog) confirmChoice() {
	name := strings.TrimSpace(d.nameField.Value())
	if name == "" {
		return
	}
	fs := d.FileSystem()
	path := name
	if !fs.IsAbs(path) {
		path = fs.Join(d.dir, path)
	}
	// A typed or Tab-completed directory name navigates into it, like the path
	// field and every standard file dialog, rather than being chosen as a file.
	if name == ".." {
		d.enterDir(fs.Parent(d.dir))
		return
	}
	// A file name cannot end in a separator, so "src/" is a directory even when
	// the probe finds nothing; loadDir surfaces the error in listErr.
	if strings.HasSuffix(name, fs.Separator()) || strings.HasSuffix(name, "/") {
		d.enterDir(path)
		return
	}
	if e, ok := d.entryFor(path); ok {
		if e.IsDir {
			d.enterDir(path)
			return
		}
	} else if _, isDir, err := fs.Exists(path); err == nil && isDir {
		// A failed probe is not "a directory": go on to finish, so a remote
		// host that cannot answer is not blocked.
		d.enterDir(path)
		return
	}
	d.finish(path)
}

// enterDir lists dir from the name field's Enter, focuses the list and clears
// the name field so the directory's name does not become the next file's.
func (d *FileDialog) enterDir(dir string) {
	d.loadDir(dir)
	d.nameField.SetValue("")
	d.setFocus(ffList)
}

// finish is the common tail of every "the user picked path" route: in Save mode
// an existing target goes through OnConfirmOverwrite, if set, before OnChoose
// fires.
func (d *FileDialog) finish(path string) {
	if d.mode == FileDialogSave && d.OnConfirmOverwrite != nil {
		// "Couldn't ask" prompts as if the file were there: a remote timeout treated as
		// "not there" would silently skip the overwrite guard, worse than a needless
		// prompt whose Yes does what the user asked anyway.
		exists, _, err := d.FileSystem().Exists(path)
		if exists || err != nil {
			d.OnConfirmOverwrite(path, func() { d.choose(path) })
			return
		}
	}
	d.choose(path)
}

func (d *FileDialog) choose(path string) {
	d.Hide()
	if d.OnChoose != nil {
		d.OnChoose(path)
	}
}

func (d *FileDialog) cancel() {
	d.Hide()
	if d.OnCancel != nil {
		d.OnCancel()
	}
}

func (d *FileDialog) activateButton() {
	if d.btnFocus == 0 {
		d.confirmChoice()
	} else {
		d.cancel()
	}
}

// FocusClipboardTargetAt implements core.ClipboardFocuser: a right-click on the
// path or name field focuses it, caret and selection untouched. The file list
// is not a text field: a right-click there is left alone.
func (d *FileDialog) FocusClipboardTargetAt(x, y int) bool {
	switch {
	case d.pathField.HitTest(x, y):
		d.setFocus(ffPath)
	case d.nameField.HitTest(x, y):
		d.setFocus(ffName)
	default:
		return false
	}
	return true
}

// FocusedClipboardTarget implements core.ClipboardHost: the path or name field
// while one has focus, nothing while the list does. The explicit nil is
// load-bearing: FocusedField returns a typed *widgets.InputField, and a nil one
// in an interface is not a nil interface, so returning it directly hands the
// caller a non-nil target whose methods dereference nil.
func (d *FileDialog) FocusedClipboardTarget() core.ClipboardTarget {
	if f := d.FocusedField(); f != nil {
		return f
	}
	return nil
}
