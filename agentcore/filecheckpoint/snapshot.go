package filecheckpoint

import "time"

// FileSnap is one file's state at the moment it was first touched in a turn.
// Content == nil means the file did not exist then, so a restore deletes it.
type FileSnap struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

// TurnCheckpoint anchors the pre-edit state of every distinct file touched
// during one user turn.
type TurnCheckpoint struct {
	Turn     int64      `json:"turn"`
	Time     time.Time  `json:"time"`
	Prompt   string     `json:"prompt"`
	MsgIndex int        `json:"msgIndex"`
	Files    []FileSnap `json:"files"`
}

// Meta is the picker-facing summary of a checkpoint (no file contents).
type Meta struct {
	Turn   int64
	Time   time.Time
	Prompt string
	Paths  []string
}
