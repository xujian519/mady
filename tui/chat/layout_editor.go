package chat

// Editor frame layout helpers — returns indices for header and editor frame
// for ChildRect queries, and resets the editor baseline row budget.

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
