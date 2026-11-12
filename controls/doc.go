// Package controls provides higher-level, reusable TUI controls:
//
//   - MenuBar / ContextMenu — application menu bar (menu_bar.go) and floating
//     right-click popup menu (context_menu.go); their shared MenuItem/Menu types
//     live in menu_item.go, and the nested submenu machinery in menu_cascade.go
//   - Toolbar — row of icon-only buttons with hover tooltips (toolbar.go)
//   - ToolRow — a panel's one-row text toolbar: layout, hit-testing, the
//     "More ▾" overflow and the gated Run, drawn by the host (tool_row.go); it hides cells by the
//     same policy as Toolbar (hideToFit, common.go)
//   - TreeView — collapsible/expandable tree with generic node data (treeview.go)
//   - ListBox — scrollable single-column list of strings (listbox.go)
//   - DataGrid — scrollable, column-aligned tabular data display, split across
//     datagrid.go, datagrid_draw.go, datagrid_input.go and datagrid_overlay.go
//   - ValueViewer — DataGrid's "Show Value" popup (value_viewer.go), a read-only
//     editor.Editor in a box
//
// The text editor is its own package, controls/editor.
//
// Controls depend on core, theme and controls/editor, not on any application types.
// The application layer passes data in and reads state out; controls never call
// back into the application directly — they fire callbacks (func values) that the
// caller wires up.
package controls
