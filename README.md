# tuikit

Widgets, dialogs, layout and charts for terminal applications on
[tcell](https://github.com/gdamore/tcell) v3: an editor, a data grid, a tree,
menus, toolbars, modal dialogs (file dialog included), a multi-page property
sheet and terminal charts. It knows no application domain and no language —
only how to draw and drive widgets on a `tcell.Screen`. It was extracted from
[goSSMS](https://github.com/radix29/gossms), its first user.

```sh
go get github.com/radix29/tuikit
```

```go
import (
    "github.com/radix29/tuikit/controls"
    "github.com/radix29/tuikit/dialogs"
    "github.com/radix29/tuikit/theme"
)
```

Go 1.27 or later. Dependencies: tcell v3 and clipperhouse/displaywidth,
nothing else (`TestTuikitImportsOnlyItsDeclaredDependencies` enforces it).

`ARCHITECTURE.md` has the long-form *why*: the mouse-gesture protocol every
host follows, the dialog stack, and the rules each of which is a bug that
shipped in an application built on tuikit.

## Package map

```
tuikit/
├── theme/      Colour palette + derived tcell.Style helpers — palette.go, styles.go
├── core/       Rect geometry, drawing primitives, string/int helpers
│             — geometry.go (Rect, SplitColumns: equal columns with a
│               gutter), screen.go, drawing.go, strutil.go (width,
│               truncate, wrap, pad; FormatThousands; FormatBytes: a
│               binary-unit size, unit chosen after rounding; FormatClock:
│               a truncating HH:MM:SS;
│               ParseFormattedNumber: their inverse, byte units scaled to
│               bytes; MatchesAny:
│               a filter box's case-insensitive substring test), mathutil.go,
│               viewport.go (Viewport: 2-D scroll offset, clamp, wheel,
│               navigation keys (ScrollKey: End keeps the column), both
│               scrollbars and their drag latches),
│               linepane.go (LinePane: a read-only line pane's scroll
│               offset, clamp, inset draw and "more" hint),
│               wrap_hanging.go (WrapHanging: wrap keeping spacing, hung
│               under a line's second column),
│               runecol.go (rune index ↔ display column), wordutil.go (word
│               boundaries for Editor and InputField), clip_screen.go
│               (ClipScreen: a tcell.Screen that drops writes outside a clip
│               rect), clipboard.go (ClipboardTarget/ClipboardHost/
│               ClipboardFocuser — the Copy/Cut/Paste
│               contract every widget package shares),
│               control.go (Focusable/Enabler/Bounded/KeyHandler/
│               MouseHandler/Overlay — the control contract hosts assert on;
│               OpenOverlay: which of a host's children has its overlay open),
│               gesture.go (Gesture: which zone of a host claimed the held
│               Button1 press, routed to until release),
│               drag.go (Drag: a drag-and-drop payload armed on a press,
│               started past a threshold, dropped on release; its ghost),
│               click.go (ClickTracker: pairs fresh presses into a
│               double-click; DoubleClickInterval, the one speed),
│               buffer.go (Buffer: an off-screen cell grid satisfying
│               tcell.Screen's drawing half — charts render into it and blit,
│               tests draw into it and read it back),
│               tabstrip.go (TabStripSegments/TabLabelWidth: the one tab-bar
│               column layout, shared by draw and hit-test code),
│               keys.go (NormalizeKey: the Ctrl+letter fold;
│               IsCtrlEnter: both Ctrl+Enter encodings), mouse.go
│               (MouseEventName: "Shift+Button1", tcell's missing
│               EventMouse.Name)
├── widgets/    InputField, DropDown, CheckBox, Button, RadioBox, Spinner — one file each;
│               progress_bar.go (DrawProgressBar: a determinate █/░ bar
│               with a trailing percentage, negative = indeterminate)
├── layout/     Panel interface, PanelManager (tabs), Splitter — panel.go,
│               panel_manager.go, splitter.go, title_bar.go (DrawTitleBar:
│               a panel's one-row title, active/inactive, optional
│               right-aligned cell)
├── dialogs/    ModalDialog (focus trap), Stack (the open dialogs, z-ordered), Properties/Alert/Message
│             (styled lines over the host's own buttons)/Confirm/TypedConfirm
│             (retype-to-confirm)/Prompt (one-line input)/Progress (Run: a
│             worker's lifecycle behind it, on the host's loop.Runner)/List (pick a
│             row, then a button)/Log (read-only newest-first log in a
│             selectable Editor; Record from any goroutine)/Text (scrollable
│             reference text, hanging-indent wrap)/FindReplace (Find and
│             Replace over an editor.Editor's search; MatchStatus, the
│             readout a host's F3 shares)/KeyDiagnostics (a Log of the
│             key and mouse events tcell decodes)/FileDialog
│             — FileDialog (every file-path prompt) is file_dialog.go (state),
│               _draw, _input, _complete (path completion), and file_system.go:
│               the FileSystem it browses (LocalFileSystem by default; Windows/
│               Posix path rules for a remote one — see ShowOpenOn/ShowSaveOn,
│               and BlockingFileSystem for the "Listing ..." repaint);
│               field_gesture.go (FieldGesture: a dialog's drag latch for a
│               gesture begun in one of its controls), focus_ring.go
│               (SetFocusIn, NextFocus, RingStep, FieldRingKey,
│               ButtonRowKey, ButtonsKey, StepButtonWrap, RingMouse,
│               ClipboardFieldAt,
│               ForwardReleaseToFocusedField …: the operations over a host
│               dialog's flat []core.Focusable ring; RingMouse is its whole
│               HandleMouse, in the one order § Mouse gestures allows),
│               field_ring.go (FieldRing: such a dialog's focus/button
│               state and its Key/Mouse/clipboard wiring in one value),
│               section_heading.go (DrawSectionHeading: a bold title over a
│               rule — propsheet's Section rows and page titles use it too),
│               task_progress.go (DrawTaskProgress: a loop.Task's bar,
│               elapsed/remaining times and message, the message wrapped over
│               every row left; TaskMessage, its default wording)
├── controls/   MenuBar+ContextMenu, Toolbar, ToolRow, TabBar, TreeView, DataGrid, ListBox
│             — menu_bar.go, context_menu.go, menu_item.go (shared types;
│               MenuItem.Checked: a toggle drawn with MenuTick at draw time),
│               clipboard_menu.go (ClipboardMenuItems: the standard
│               Cut/Copy/Paste/Select All right-click menu),
│               choice_items.go (ChoiceItems: a pick-one selector's menu,
│               the value in force marked with ChoiceMark),
│               check_items.go (CheckItems: a pick-many checklist's
│               KeepOpen rows, ticked with CheckMark; ContextMenu.SetRefresh
│               redraws them in place),
│               menu_box.go (one box per menu level, root and submenus:
│               draw, scroll, hit-test, keys for both hosts),
│               toolbar.go (ToolbarButton.On: a toggle painted selected
│               while on, at a fixed width), tool_row.go (a panel's text toolbar: layout,
│               hit-test, "More ▾"; drawn by the host), tab_bar.go (TabBar:
│               a one-row strip of plain tabs plus an optional right-aligned
│               cell, draw and hit-test over TabStripSegments), treeview.go (with an
│               optional top-border action, SetTitleButton), listbox.go;
│               DataGrid: datagrid.go (state/source/widths), _draw, _input,
│               _overlay (right-click menu, "Show Value"), _sort (a host's
│               column sort: PermutedSource, SetSourceReordered, SortMenuItems),
│               _textcell (a pre-rendered text cell read for sorting and
│               SetAutoAlign: TextCellKey, CompareTextCells, ClassifyTextCell);
│               value_viewer.go (ValueViewer: the modal read-only popup
│               "Show Value" opens; a host can hold its own)
│   └── editor/ Editor — document.go (the one text-mutation chokepoint; its
│               version counter keys the syntax/wrap/width caches),
│               line_buffer.go (builds a document off the UI goroutine for an
│               O(1) SetLineBuffer), log_feed.go (LogFeed: a newest-first
│               capped log for a read-only Editor, synced by PrependLines so
│               the reader keeps their selection), editor.go (state/options),
│               editor_undo.go (span deltas, capped by steps and bytes),
│               editor_search.go (the
│               one regexp engine for Find/Replace/Replace All), editor_block.go
│               (column selection + rectangular clipboard), editor_selection.go,
│               _draw, _wrap, _input, _actions, editor_completion.go (popup;
│               word rule and trigger are the host's: SetWordStart,
│               SetCompletionTrigger); xml_highlighter.go (planview XML tab),
│               json_highlighter.go (JSON cell panel; stateless per line). A
│               host language's highlighter keeps its line states on the
│               Document through LineStateAt; Enter's extra indent is
│               SetIndentRule's
├── charts/     Terminal charts from generic series data
│             — rendered into a core.Buffer, then blitted/scrolled; scale.go (nice-number
│               scales, ticks, formatting), glyph.go (eighth-block ramps),
│               axis.go, legend.go, common.go (Series, stacked-run composition),
│               history.go, stacked_history.go, barchart.go, stacked_bar.go,
│               vbar.go, kpi.go, panel_title.go (a chart panel's heading row),
│               section_bar.go (a dashboard section's title strip with
│               right-aligned KPIs), plot_hit.go (PlotHit: map a click on a
│               drawn plot back to a bucket and the value drawn there),
│               readout.go (the box a click on a chart pins: size, placement,
│               draw; DrawAxisCallout names its moment on the time axis),
│               pin.go (Pin: the pinned readout's life — pin a bucket, not a
│               column; follow it as samples push it left; close on prune)
├── clipboard/  The system clipboard behind a host's Copy/Paste — native.go
│               (Write/Read through pbcopy, wl-copy, xclip or xsel),
│               native_windows.go (the Win32 CF_UNICODETEXT path; tuikit's one
│               GOOS-suffixed file), terminal.go (TerminalLikely: did an OSC 52
│               write probably land), paste.go (PasteBuffer: a bracketed
│               paste's keys as one text; StripBOMs), dispatcher.go
│               (Dispatcher: Copy/Paste/HandleEvent over the above —
│               newest copy wins, a paste aimed once, bracketed paste as one
│               edit, the OSC 52 reply). Write/Read are synchronous; the
│               Dispatcher runs them through the host's Go and Post
├── loop/       Async plumbing (UI-goroutine and background sides) — poster.go (Poster: Post a
│               callback from any goroutine and wake the event loop through
│               EventQ(); Drain at the top of every iteration; Close orders the
│               screen's Fini against a wake in flight), latest.go (Latest:
│               a latest-only load's token + cancel — Begin supersedes and
│               cancels the run in flight, Done drops a stale result),
│               runner.go (Runner: the background side — Go/GoRepair/Recover
│               start or guard a goroutine so a panic is reported, not fatal;
│               GoOnce a single-flight run whose latch a panic releases;
│               FanOut is a bounded pool recovering per item; Tick a
│               spinner's redraw clock, stopped by closing done),
│               tasks.go (Tasks/Task: non-modal background tasks — Start
│               a task and its context, Progress/Done post from the worker,
│               Finish settles it with the cancelled-vs-failed rule
│               ProgressDialog.Run also keeps; ErrTaskPanicked; Keep caps
│               the history, never evicting a running task).
│               ARCHITECTURE.md § Threading
├── tuikittest/ Test kit, imported by _test files only — screen.go (Screen: a
│               core.Buffer with Find/RowOf/Snapshot/SetSize),
│               mockscreen.go (NewMockScreen: tcell's own screen on a mock
│               terminal), events.go (Key/Rune/Mouse/Press/Release builders)
├── propsheet/  PropertySheet — multi-page editable properties dialog framework
│             — doc.go, common.go, rows.go (Section/Note/Hint/Static),
│               textrow.go, choicerows.go (Check/Select/Radio),
│               preservingselect.go (SelectPreserving/PreservingItems: a
│               Select that shows a value its list lacks, with a stand-in
│               for none that never reads back as a value), buttonsrow.go,
│               gridrow.go, editorrow.go, togglegrid.go, griddetail.go
│               (BindGridDetail: detail rows following a grid's selection),
│               listeditor.go (ListEditor: BindGridDetail + Add/Remove),
│               pendingedits.go (PendingEdits: the rows as loaded and as
│               edited behind a ListEditor — add/remove/restore/swap,
│               dirty, revert, name checks under the host's equality), celltoggle.go
│               (BindCellToggle/NewCellToggleGrid: activate a cell to change
│               its row), filterindex.go (FilterIndex: a filter box's
│               view over a grid's list — rows map through it; its
│               FilterRow's onFilter is where the host clears the detail
│               on an empty result), staticblock.go (StaticBlock: detail rows filled
│               from a selection, cleared as one), form.go; sheet.go
│               (state/pages), sheet_draw.go, sheet_input.go, sheet_clipboard.go
└── examples/   Runnable programs, not library packages (nothing imports them)
    └── spindemo/  every widgets.Spinner animated side by side, for picking
                   one by eye — go run ./examples/spindemo
```

Every sub-package: one file per type or tight group, plus `doc.go`.

## Dependency direction

One-way; nothing imports upward, nothing imports an application. Each package imports
`core` and `theme` (`core` only `theme`, `theme` only `tcell`), plus:

```
widgets          —
charts           —
controls/editor  —
controls         → controls/editor
layout           —
dialogs          → widgets, controls/editor, loop
propsheet        → dialogs, controls, controls/editor, widgets
clipboard        imports no tuikit package, not even core/theme (tcell only)
loop             imports no tuikit package, not even core/theme (tcell only)
examples/*       → whatever they demonstrate; nothing imports them
```

- **layout** is a leaf over `core`/`theme`: `PanelManager`'s tab bar takes its
  geometry from `core.TabStripSegments`, as every other tab bar built on
  tuikit does; nothing in tuikit imports `layout`.
- **controls/editor** is a leaf too: `controls` reaches it only through
  `ValueViewer` (DataGrid's "Show Value"), and `dialogs` only through
  `LogDialog`, each of which embeds an `Editor`.
- **propsheet** is the top, composing `dialogs.ModalDialog`,
  `controls.DataGrid`/`ListBox`, `editor.Editor` (EditorRow) and `widgets`.
- **clipboard** is outside the widget graph: no widget calls it. A widget
  hands text over through `core.ClipboardTarget`; the host moves it to and
  from the system clipboard with this package.
- **loop** is outside the widget graph too: no widget posts, wakes or starts a goroutine. It is
  host plumbing, here because § Threading dictates the protocol and every
  tcell v3 host otherwise rewrites its bug-dense part. Its one tuikit
  importer is `dialogs`, for `ProgressDialog.Run`, which takes the host's
  `Runner` and `Poster` as arguments, as `clipboard.Dispatcher` takes its `Go`
  and `Post`, and for `DrawTaskProgress`, which only reads a `Task`.
- **No language lives here.** A host's language plugs in through `Editor`'s
  hooks: a highlighter keeping its line states through `LineStateAt`,
  `SetIndentRule`, `SetWordStart`, `SetCompletionTrigger`, `SetLineComment`
  (goSSMS's T-SQL editor is built this way). The XML and JSON highlighters
  are here because they are language-neutral.

## Design principles

**No upward calls.** Controls talk outward only via callbacks (`OnExpand`,
`OnSelect`, `OnClick`, `OnConfirm`, …) that the host wires.

**Geometry via `core.Rect`.** Widgets store a `core.Rect` and expose
`SetBounds(x, y, w, h)`; containers (`PanelManager`, `Splitter`) compute child
rects and hand them down. Two shapes differ, both one row high: the
`widgets` leaf inputs (`Button`, `CheckBox`, `DropDown`, `InputField`,
`RadioBox`) take `SetBounds(x, y)`, their width fixed by their label or by
`SetWidth`; `MenuBar` and `Toolbar` take `SetBounds(x, y, w)`.

**Self-contained state**, read through getters (`Value()`, `Checked()`,
`Selected()`); a host never pushes into private fields.

**An animation is a function of elapsed time, not a timer.** `widgets.Spinner`
(catalogue: `SpinnerBraille`, `SpinnerCodex`, …; `widgets.Spinners`,
`SpinnerByName`) has no start time or goroutine: `Frame(elapsed)`/
`FrameSince(start)`, redrawn from the host's clock. All frames of a Spinner
share one display width, or a narrower frame leaves the previous one's tail
(`TestSpinnerFramesAreUniformWidth`). `go run ./examples/spindemo`
shows them all.

**One control contract, as small interfaces in `core`** (`control.go`):
`Focusable` (`Focus(bool)`/`Focused() bool`), `Enabler`
(`SetEnabled`/`Enabled`), `Bounded` (`Bounds() core.Rect`), `KeyHandler`,
`MouseHandler`, `Overlay` (below) — plus `ClipboardTarget`. A host (a focus
ring, `propsheet.Form`, an application dialog) type-asserts on these instead
of switching over widget types; each package's `control_test.go` pins which
control implements which. `Focus(bool)` is the one focus spelling:
`SetActive(bool)` means only `layout.Activatable`, and
`PanelManager.SetActiveIndex(i)` switches tabs. An input widget reports user
edits through an `OnChange` field (`InputField`, `CheckBox`, `RadioBox`,
`DropDown`), never fired by its setters, so a host need not compare values
after every key.

**Optional capability interfaces.** `layout.Panel` requires only `SetBounds`,
`Draw`, `HandleKey`, `HandleMouse`, `Title`. `layout.Activatable`
(`SetActive(bool)`) is called by `PanelManager` on every switch if
implemented; `layout.Disposable` (`Close()`) is called by the host before
`RemovePanel` for panels owning a read or connection.

**Row data behind an interface.** `controls.DataGrid` takes a `RowSource`
(`Len()`, `Row(i)`); `SetData` wraps `SetSource` with a `SliceRowSource`.
Column widths sample only the first `colWidthSampleRows`, so large or streamed
sources scale. A source may also implement `RowKindSource` to mark rows as
group headers (`RowGroup` — cell 0 a label from the left edge, unscrolled and
not sampled for widths, spilling across empty cells up to the first non-empty
one, which draws in its column like an ordinary cell; expanding one is the
host's rebuild) or marked rows (`RowMarked`, bookmarks), and `NullSource`
(`IsNull(row, col)`) to mark SQL NULL cells, which draw dimmed and skip
`OnShowValue`. The grid never reads NULL from the text: a cell reading "NULL"
may be the string. goSSMS's query result set is a `RowSource` and a
`NullSource`, streamed in as rows arrive. A host's column sort (the grid draws
the arrow and reports the header click, never sorts) shows its order through
`PermutedSource` and swaps it in with `SetSourceReordered`, which keeps the
cursor on its row, not its index.

**Display width, not bytes or runes.** `core.DisplayWidth(s)` (via
`clipperhouse/displaywidth`) is the one answer to "how many columns", handling
wide CJK and grapheme clusters; `core.DrawText`, `DrawTextClipped`,
`DrawTextRight`, `Truncate`, `TruncateLeft` (keeps the tail, for paths),
`PadRight` build on it. Anything positioned after a label uses
`core.DisplayWidth(label)`, never `len(label)`, or drawing and
hit-testing desync. Rune-indexed text (`Editor`, `InputField` cursors,
selections, wrap segments) converts via `core/runecol.go` only — `RunesWidth`,
`ColumnOfRune`, `RuneIndexAtColumn`/`ClusterAtColumn`, and `GraphemeAt`,
`NextGrapheme`, `PrevGrapheme` for drawing and cursor steps. They measure by
grapheme cluster, as tcell does: summing rune widths made `❤️` one column, a
flag four and a ZWJ family six, so the glyph overwrote the next cell. Draw a
cluster in one `Put`, move the cursor over it whole, and never
size text with `RuneWidth` alone — it is right only for a lone rune. Until
2026-08-02 both widgets treated rune index as column, and a CJK/emoji character
shifted the rest of its line.

**Async state as data, not goroutines — `propsheet.PropertySheet`.** A
multi-page dialog (page list, a `Form` of `Row`s, OK/Cancel/Apply/Script
Changes) whose pages load lazily. It never spawns a goroutine: it calls
`OnLoadPage(page, seq)`; the host fetches and replies with
`SetPageForm(page, seq, form)`/`SetPageError(page, seq, err)`. `seq` is
sheet-wide, monotonic, never reset by `SetPages`; a stale `seq` is ignored
(`SetPageForm` returns false). **Both must be called on the UI goroutine** —
no locking. Rows share a small `Row` interface plus optional capabilities
(`Editable`, `Copyable`, `KeyHandler`, … — `propsheet/common.go`), so a new
row kind never touches `Form`. **A row that draws a control also implements
`ReadOnlyDrawer`**: under `Form.SetReadOnly`, a row still drawing `[value]` or
`[ ]` looks like a field refusing input. **A read-only form still lets its
`Browsable` rows (grids) take focus and clicks** — they go browse-only
(`DataGrid.SetBrowseOnly`: select, scroll, copy, never `OnActivateCell`), so
detail rows that follow a selection stay readable; the form is never `Dirty()`.

**Overlays are drawn last and get first refusal of input.** A widget whose open
state floats outside its rect — `DropDown`'s list, `DataGrid`'s menu/"Show
Value" popup (a `ValueViewer`), `Editor`'s completion popup, `MenuBar`'s
dropdown, a `ContextMenu` — implements `core.Overlay`: `DrawOverlay(s)`, which the host
calls after everything sharing that space, and `OverlayActive()`, which the
host checks first in its own `HandleKey`/`HandleMouse`, or input goes to
whatever sits underneath (`ARCHITECTURE.md` § Overlays). A hover hint (`Toolbar`, `TreeView`'s
title button) is an `Overlay` whose `OverlayActive` is always false: it draws
last but takes no input.

**Theming is global but swappable**: `theme.Active()`; `theme.SetPalette(p)`
at startup or on a theme switch reskins everything. `Active()` returns the
live palette: read it, never write through it.

**Package-level settings belong to the UI goroutine.** None has a lock; set
each before the first Draw (or, for a runtime change, between frames):

- `theme.SetPalette` — the palette every `Style*` and `Active()` reads.
- `editor.SetDefaultIndentWidth` — the width new Editors start with.
- `propsheet.ApplyingSpinner` — the button-row busy indicator.
- `controls.Strings`, `dialogs.Strings`, `propsheet.Strings` — the library's
  own UI text (button labels, default dialog titles, FileDialog's labels and
  columns, DataGrid's status and menu, ToolRow's "More", the sheet's hints,
  page states and `Int` validator). Controls dispatch on IDs or positions, never on this text,
  so a host may translate it. App-specific text is a field instead:
  `layout.PanelManager.EmptyText`, `dialogs.ProgressDialog.CancellingText`
  (seeded from `dialogs.Strings`), `PropertySheet.SetHints`.

## How an application uses it

The host defines its domain types, wires controls with callbacks that load its
data, embeds `dialogs.ModalDialog` in its own dialogs, and implements
`layout.Panel` for its panes. In goSSMS, for example, the Object Explorer owns
the tree model and projects it into a flat `[]controls.TreeNode`; all walking,
scrolling and expand/collapse lives once in `controls.TreeView`. The host owns
the event loop and the one UI goroutine (`ARCHITECTURE.md` § Threading).

## Adding a new control

1. Pick the package: leaf input → `widgets`; layout primitive → `layout`;
   self-contained modal → `dialogs`; bigger stateful/scrolling → `controls`.
2. Depend only on `core` and `theme` unless building on another tuikit package.
   A new package, or a new edge between two, goes in § Dependency direction
   and in `tuikitLayers` (`core/imports_test.go`), which enforces it.
3. Interactive controls expose `SetBounds`, `Bounds`, `Draw(tcell.Screen)`,
   `HandleKey`, `HandleMouse`, `Focus`/`Focused` if they take focus, plus plain
   getters — the `core` control interfaces; add the new type to its package's
   `control_test.go`. A value the user edits gets an `OnChange` field fired by
   user edits only.
4. If it can be switched off: `SetEnabled(bool)`/`Enabled() bool` with
   `InputField`'s contract (shared by `Button`, `CheckBox`, `RadioBox`) —
   draws greyed (`theme.StyleControlDisabled`; `StyleButtonDisabled` for
   buttons) **and** refuses input, keeps its focus-ring slot, and its own
   setters still work (pinning a value the server requires is the usual reason
   to disable).
5. Never import an application package.

## Adding a new dialog in an application

```go
type MyDialog struct {
    dialogs.ModalDialog
    // your fields...
}

func NewMyDialog() *MyDialog {
    d := &MyDialog{}
    d.InitModal("My Dialog", 50, 12)   // no screen: the Stack gives it one
    return d
}

func (d *MyDialog) Draw(s tcell.Screen) {
    if !d.Visible() { return }
    d.DrawBase(s)              // overlay + box + title
    // ...draw your content inside d.InnerRect()...
    d.DrawSeparator(s)
    d.DrawButtons(s, []string{"OK", "Cancel"}, activeIdx)
}

func (d *MyDialog) HandleMouse(ev *tcell.EventMouse) bool {
    if !d.Visible() { return false }
    if d.ConsumeOutsideClick(ev) { return true }  // focus trap
    if i := d.ButtonClicked(ev, []string{"OK", "Cancel"}); i >= 0 {
        // handle button i
    }
    return true
}
```

Give a `dialogs.Stack` the screen once (`SetScreen`), register every dialog
once — up front, or when the host first builds it; registering hands it the
screen it centres on — and let the Stack host them: call
`Sync` before routing an event and before drawing, then `Draw`, `Relayout` (on
resize), `HandleKey`/`HandleMouse` (they report whether a dialog consumed the
event) and `ClipboardTarget`. **Clip contract:** a dialog clamped to a small
terminal confines its content only when drawn on a `core.ClipScreen` —
`Stack.Draw` supplies one; a host drawing dialogs itself must too.
`DrawBase` resets that screen's clip before narrowing it, so dialogs sharing
one never inherit each other's.

`dialogs.FileDialog` is the same pattern inside tuikit; it reaches disk only
via its `FileSystem` interface, so a host can point it at another machine
(goSSMS's Backup/Restore browse the database server's disks) while other
callers stay local. A dialog with a text field also follows
`ARCHITECTURE.md` § dialogs.FieldGesture.

## Development

`go build ./... && go vet ./... && go test ./...`; `staticcheck ./...` stays
clean (`deadcode` lists exported API a library keeps, so it is not a cleanup
list here). goSSMS builds against a sibling checkout
(`replace github.com/radix29/tuikit => ../tuikit` in its `go.mod`), so a
change here is also driven in goSSMS's built binary before it is called done.
