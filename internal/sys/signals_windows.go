//go:build windows

package sys

import "os"

func terminateProcess(p *os.Process) error {
	return p.Kill()
}

func pauseProcess(_ *os.Process) bool {
	return false
}

func resumeProcess(_ *os.Process) bool {
	return false
}
