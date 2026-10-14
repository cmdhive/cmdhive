//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// PTY is one child process attached to a pseudo terminal. Output arrives on
// Read exactly as a terminal emulator expects it.
type PTY struct {
	f         *os.File
	cmd       *exec.Cmd
	closeOnce sync.Once
}

// StartPTY launches argv in dir. The child leads its own session, so Close
// takes everything it started with it (claude spawns shells for its tools,
// and those must not outlive an account switch); killTree is therefore
// always in effect here.
func StartPTY(argv []string, dir string, env []string, cols, rows int, killTree bool) (*PTY, error) {
	if len(argv) == 0 {
		return nil, errors.New("nothing to run")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = withTerm(env)
	f, err := pty.StartWithSize(cmd, winsize(cols, rows))
	if err != nil {
		return nil, err
	}
	return &PTY{f: f, cmd: cmd}, nil
}

// withTerm adds TERM, which an app started from the Dock does not have.
func withTerm(env []string) []string {
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "TERM=" {
			return env
		}
	}
	return append(env, "TERM=xterm-256color")
}

func winsize(cols, rows int) *pty.Winsize {
	if cols < 2 {
		cols = 2
	}
	if rows < 1 {
		rows = 1
	}
	return &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
}

func (p *PTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *PTY) Write(b []byte) (int, error) { return p.f.Write(b) }

func (p *PTY) Resize(cols, rows int) error { return pty.Setsize(p.f, winsize(cols, rows)) }

// Wait blocks until the process exits and returns its exit code.
func (p *PTY) Wait() (uint32, error) {
	err := p.cmd.Wait()
	code := p.cmd.ProcessState.ExitCode()
	if code < 0 { // ended by a signal
		code = 1
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		err = nil
	}
	return uint32(code), err
}

// Close kills the process and everything it started, and releases the
// terminal. Safe to call more than once and from any goroutine.
func (p *PTY) Close() {
	p.closeOnce.Do(func() {
		if p.cmd.Process != nil {
			syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
			p.cmd.Process.Kill()
		}
		p.f.Close()
	})
}

// release is for the Windows process handle; nothing to free here.
func (p *PTY) release() {}
