// Package filecheckpoint provides a git-free snapshot-based edit safety net.
// Before a writer tool (edit, write_file, patch, delete, move) changes a file,
// the agent records the file's pre-edit content here, keyed to the current user
// turn. A frontend can then rewind the workspace to an earlier turn.
//
// Only edit-tool changes are tracked — bash side effects are not (a shell
// command's targets can't be known in advance). Snapshots live in memory and
// optionally persist to a checkpoint directory.
package filecheckpoint
