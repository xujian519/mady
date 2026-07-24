package tools

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// navigationTimeout returns the effective navigation timeout.
// If the given timeout is greater than 60s, it is used as-is;
// otherwise a default 60s timeout is returned.
func navigationTimeout(commandTimeout time.Duration) time.Duration {
	if commandTimeout > 60*time.Second {
		return commandTimeout
	}
	return 60 * time.Second
}

// isDeadlineError reports whether err wraps context.DeadlineExceeded.
func isDeadlineError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// navigationTimeoutError returns a descriptive error for a navigation timeout.
func navigationTimeoutError(url string, timeout time.Duration) error {
	return fmt.Errorf("navigation timed out after %s while opening %s. The page may be slow, blocked, or still loading; try again, use browser_snapshot if the page partially loaded, or raise the browser command timeout", timeout.Round(time.Second), url)
}
