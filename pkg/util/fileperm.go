package util

import "os"

// Common file permission constants for use across the application.
// Centralizing these makes security audits easier.
const (
	// DefaultFilePerm is the default permission for files created by the application.
	DefaultFilePerm os.FileMode = 0o600

	// DefaultDirPerm is the default permission for directories created by the application.
	DefaultDirPerm os.FileMode = 0o750

	// ReadWriteFilePerm is used for files that need to be written and read.
	ReadWriteFilePerm os.FileMode = 0600

	// ExecutableDirPerm is used for directories that need execute permission.
	ExecutableDirPerm os.FileMode = 0755
)
