//go:build !windows

package sys

import (
	"os"
	"syscall"
)

func terminateProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

func pauseProcess(p *os.Process) bool {
	return p.Signal(syscall.SIGSTOP) == nil
}

func resumeProcess(p *os.Process) bool {
	return p.Signal(syscall.SIGCONT) == nil
}
