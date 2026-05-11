package internal

import (
	"time"
)

const defaultTimeout = time.Second * 15

// FallbackTimeout returns the given duration unless it's non-positive,
// in which case it defaults to a predefined timeout.
func FallbackTimeout(v time.Duration) time.Duration {
	if v <= 0 {
		return defaultTimeout
	}
	return v
}
