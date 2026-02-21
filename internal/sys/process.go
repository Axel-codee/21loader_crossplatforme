package sys

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type ProcessError struct {
	Command string
	Status  int
}

func (e ProcessError) Error() string {
	return fmt.Sprintf("la commande a echoue (%d): %s", e.Status, e.Command)
}

type Runner struct {
	mu     sync.Mutex
	active *exec.Cmd
	cancel context.CancelFunc
	stdin  io.WriteCloser
}

type RunOptions struct {
	Executable    string
	Args          []string
	WorkingDir    string
	StandardInput string
	OnOutput      func(string)
	CaptureOutput bool
}

func (r *Runner) Run(parent context.Context, opt RunOptions) (string, error) {
	if opt.CaptureOutput == false {
		// no-op; still streams output
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	executable := opt.Executable
	args := opt.Args
	if !strings.Contains(executable, string(filepath.Separator)) {
		args = append([]string{executable}, args...)
		executable = "env"
	}

	cmd := exec.CommandContext(ctx, executable, args...)
	if opt.WorkingDir != "" {
		cmd.Dir = opt.WorkingDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	var stdin io.WriteCloser
	if opt.StandardInput != "" {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return "", err
		}
	} else {
		cmd.Stdin = os.Stdin
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	r.mu.Lock()
	r.active = cmd
	r.cancel = cancel
	r.stdin = stdin
	r.mu.Unlock()

	var buf bytes.Buffer
	var bufMu sync.Mutex
	emit := func(chunk string) {
		if chunk == "" {
			return
		}
		if opt.CaptureOutput {
			bufMu.Lock()
			buf.WriteString(chunk)
			bufMu.Unlock()
		}
		if opt.OnOutput != nil {
			opt.OnOutput(chunk)
		}
	}
	stream := func(rd io.Reader, wg *sync.WaitGroup) {
		defer wg.Done()
		readBuf := make([]byte, 64*1024)
		pending := make([]byte, 0, 4*1024)
		for {
			n, readErr := rd.Read(readBuf)
			if n > 0 {
				emittedBySeparator := false
				for _, b := range readBuf[:n] {
					if b == '\n' || b == '\r' {
						if len(pending) > 0 {
							emit(string(pending) + "\n")
							pending = pending[:0]
							emittedBySeparator = true
						}
						continue
					}
					pending = append(pending, b)
				}
				// Some tools (including yt-dlp on certain platforms/configs) emit progress
				// updates without line separators. Flush chunk-wise to preserve live logs.
				if !emittedBySeparator && len(pending) > 0 {
					emit(string(pending))
					pending = pending[:0]
				}
			}
			if readErr != nil {
				if len(pending) > 0 {
					emit(string(pending))
				}
				if errors.Is(readErr, io.EOF) {
					return
				}
				return
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go stream(stdout, &wg)
	go stream(stderr, &wg)

	if stdin != nil {
		_, _ = io.WriteString(stdin, opt.StandardInput)
		_ = stdin.Close()
	}

	waitErr := cmd.Wait()
	wg.Wait()

	r.mu.Lock()
	if r.active == cmd {
		r.active = nil
		r.cancel = nil
		r.stdin = nil
	}
	r.mu.Unlock()

	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			status := exitErr.ExitCode()
			if status == -1 && ctx.Err() != nil {
				return buf.String(), ctx.Err()
			}
			return buf.String(), ProcessError{Command: strings.Join(append([]string{opt.Executable}, opt.Args...), " "), Status: status}
		}
		return buf.String(), waitErr
	}
	return buf.String(), nil
}

func (r *Runner) CancelCurrentProcess() {
	r.mu.Lock()
	cmd := r.active
	cancel := r.cancel
	r.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = terminateProcess(cmd.Process)
	}
	if cancel != nil {
		cancel()
	}
}

func (r *Runner) PauseCurrentProcess() bool {
	r.mu.Lock()
	cmd := r.active
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	return pauseProcess(cmd.Process)
}

func (r *Runner) ResumeCurrentProcess() bool {
	r.mu.Lock()
	cmd := r.active
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	return resumeProcess(cmd.Process)
}
