//go:build !darwin && !linux && !windows

package providers

import (
	"fmt"
	"io"
)

func queuedACPRealPipeEvidence(io.Reader) (queued, eof bool, err error) {
	return false, false, fmt.Errorf("safe ACP pipe settlement inspection is unavailable on this platform")
}
