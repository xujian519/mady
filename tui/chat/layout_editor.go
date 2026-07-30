package chat

// This file defines the chatLayout — the root Component that stacks header,
// chat history, autocomplete, loader, editor (bordered), footer, and status
// bar via the declarative Flex layout. It also owns the input router
// (Update), translating keys/mouse/paste into the right child action
// (scrolling, copy-vs-interrupt, autocomplete, image paste), and the
// copy/copy-shortcut helpers.

// Returns the indices for header and editor frame for ChildRect queries.
// resetEditorBaseline restores the editor to its baseline row budget so a
// previous render's OnAllocate shrink does not contaminate the next
// natural-height measurement. Both buildFlex and recalcMaxRows measure the
// editor's natural height and must call this first.
func (l *chatLayout) resetEditorBaseline() {
	if l.editorMaxRows > 0 {
		if ed, ok := l.editor.(maxRowsSetter); ok {
			ed.SetMaxRows(l.editorMaxRows)
		}
	}
}
