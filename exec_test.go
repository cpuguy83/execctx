package execctx

import (
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestNilHandler(t *testing.T) {
	cmd := exec.Command("sleep", "99999")

	ctx, cancel := context.WithCancel(context.Background())
	c := FromCmd(ctx, cmd, nil)
	assert.NilError(t, c.Start())

	cancel()
	assert.ErrorContains(t, c.Wait(), "killed")
	assert.Assert(t, cmd.ProcessState.ExitCode() != 0)
}

func TestCustomHandler(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "cat -; sleep 99999")

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	cmd.Stdin = stdinR

	stdoutR, stdoutW := io.Pipe()
	defer stdoutR.Close()
	cmd.Stdout = stdoutW

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	handlerDone := make(chan struct{})
	c := FromCmd(ctx, cmd, func() {
		defer close(handlerDone)
		stdinW.Write([]byte("hello\n"))
		stdinW.Close()

		buf := make([]byte, 6)
		n, err := stdoutR.Read(buf)
		assert.NilError(t, err)
		assert.Equal(t, string(buf[:n]), "hello\n")
		select {
		case <-done:
			t.Fatal("Process exited unexpectedly")
		default:
		}

		cmd.Process.Kill()
	})

	assert.NilError(t, c.Start())
	t.Log("command started")
	cancel()

	go func() {
		t.Log(c.Wait())
		t.Log("command exited")
		stdinR.Close()
		stdoutW.Close()
		close(done)
	}()

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		t.Fatal("timeout waiting for process to exit")
	case <-done:
		assert.Assert(t, cmd.ProcessState.ExitCode() != 0)
	}
	<-handlerDone
}
