// Package editor provides Editor, the multi-line text editor control, and the
// pieces it is built from:
//
//   - Editor — state and options (editor.go), drawing (editor_draw.go), soft
//     wrap (editor_wrap.go), input (editor_input.go), edit actions
//     (editor_actions.go), selection (editor_selection.go), column selection
//     and its rectangular clipboard (editor_block.go), undo (editor_undo.go),
//     the one search engine for Find/Replace (editor_search.go) and the
//     completion popup (editor_completion.go)
//   - Document — the buffer, the one text-mutation chokepoint; its version
//     counter keys the syntax/wrap/width caches (document.go)
//   - LineBuffer — builds a Document off the UI goroutine (line_buffer.go)
//   - XMLHighlighter, JSONHighlighter — the built-in, language-neutral
//     highlighters (xml_highlighter.go, json_highlighter.go)
//
// A host language plugs in through SetHighlighter, LineStateAt, SetIndentRule,
// SetWordStart and SetCompletionTrigger; nothing here knows any language.
//
// The package depends on core and theme only, never on controls: controls'
// ValueViewer embeds an Editor, not the other way round.
package editor
