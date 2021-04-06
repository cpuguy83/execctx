package execctx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
)

// Cmd wraps an os/exec.Cmd to enable custom handling of context cancellations
// The stdlib `exec.CommandContext` only supports sending SIGKILL when the a
// context is cancelled, this package allows you to pass a custom handler.
//
// Create one with `FromCmd`
type Cmd struct {
	ctx      context.Context
	cancel   func()
	cmd      *exec.Cmd
	waitDone chan struct{}
}

// FromCmd wraps an os/Exec.Cmd with custom handling for when the provided
// context is cancelled.
// If the provided cancel function is nil, the process
// will be killed with SIGKILL
func FromCmd(ctx context.Context, cmd *exec.Cmd, cancel func()) *Cmd {
	return &Cmd{ctx: ctx, cmd: cmd, cancel: cancel, waitDone: make(chan struct{})}
}

// Wait waits for the command to exit
func (c *Cmd) Wait() error {
	err := c.cmd.Wait()
	close(c.waitDone)
	return err
}

// Start starts the command
func (c *Cmd) Start() error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
	}

	if err := c.cmd.Start(); err != nil {
		return err
	}

	go func() {
		select {
		case <-c.ctx.Done():
			if c.cancel == nil {
				c.cmd.Process.Kill()
				return
			}
			c.cancel()
		case <-c.waitDone:
		}
	}()

	return nil
}

// Run starts the command and waits for it to exit
func (c *Cmd) Run() error {
	err := c.Start()
	if err != nil {
		return nil
	}

	return c.Wait()
}

// CombinedOutput runs the command, waits for it to exit, and returns
// the combined stdout and stderr of the command.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var b bytes.Buffer
	c.cmd.Stdout = &b
	c.cmd.Stderr = &b
	err := c.Run()
	return b.Bytes(), err
}

// Output runs the command, waits for it to exit, and returns the
// stdout of the command.
func (c *Cmd) Output(ctx context.Context) ([]byte, error) {
	if c.cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	c.cmd.Stdout = &stdout

	captureErr := c.cmd.Stderr == nil
	if captureErr {
		c.cmd.Stderr = &prefixSuffixSaver{N: 32 << 10}
	}

	err := c.Run()
	if err != nil && captureErr {
		if ee, ok := err.(*exec.ExitError); ok {
			ee.Stderr = c.cmd.Stderr.(*prefixSuffixSaver).Bytes()
		}
	}
	return stdout.Bytes(), err

}

func (c *Cmd) String() string {
	return c.cmd.String()
}

// prefixSuffixSaver is an io.Writer which retains the first N bytes
// and the last N bytes written to it. The Bytes() methods reconstructs
// it with a pretty error message.
//
// This is copied from stdlib os/exec
type prefixSuffixSaver struct {
	N         int // max size of prefix or suffix
	prefix    []byte
	suffix    []byte // ring buffer once len(suffix) == N
	suffixOff int    // offset to write into suffix
	skipped   int64

	// TODO(bradfitz): we could keep one large []byte and use part of it for
	// the prefix, reserve space for the '... Omitting N bytes ...' message,
	// then the ring buffer suffix, and just rearrange the ring buffer
	// suffix when Bytes() is called, but it doesn't seem worth it for
	// now just for error messages. It's only ~64KB anyway.
}

func (w *prefixSuffixSaver) Write(p []byte) (n int, err error) {
	lenp := len(p)
	p = w.fill(&w.prefix, p)

	// Only keep the last w.N bytes of suffix data.
	if overage := len(p) - w.N; overage > 0 {
		p = p[overage:]
		w.skipped += int64(overage)
	}
	p = w.fill(&w.suffix, p)

	// w.suffix is full now if p is non-empty. Overwrite it in a circle.
	for len(p) > 0 { // 0, 1, or 2 iterations.
		n := copy(w.suffix[w.suffixOff:], p)
		p = p[n:]
		w.skipped += int64(n)
		w.suffixOff += n
		if w.suffixOff == w.N {
			w.suffixOff = 0
		}
	}
	return lenp, nil
}

// fill appends up to len(p) bytes of p to *dst, such that *dst does not
// grow larger than w.N. It returns the un-appended suffix of p.
func (w *prefixSuffixSaver) fill(dst *[]byte, p []byte) (pRemain []byte) {
	if remain := w.N - len(*dst); remain > 0 {
		add := minInt(len(p), remain)
		*dst = append(*dst, p[:add]...)
		p = p[add:]
	}
	return p
}

func (w *prefixSuffixSaver) Bytes() []byte {
	if w.suffix == nil {
		return w.prefix
	}
	if w.skipped == 0 {
		return append(w.prefix, w.suffix...)
	}
	var buf bytes.Buffer
	buf.Grow(len(w.prefix) + len(w.suffix) + 50)
	buf.Write(w.prefix)
	buf.WriteString("\n... omitting ")
	buf.WriteString(strconv.FormatInt(w.skipped, 10))
	buf.WriteString(" bytes ...\n")
	buf.Write(w.suffix[w.suffixOff:])
	buf.Write(w.suffix[:w.suffixOff])
	return buf.Bytes()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
