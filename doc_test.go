package execctx

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func ExampleFromCmd_nilHandler() {
	cmd := exec.Command("sleep", "99999")
	ctx, cancel := context.WithCancel(context.Background())

	c := FromCmd(ctx, cmd, nil)
	if err := c.Start(); err != nil {
		panic(err)
	}

	cancel()
	c.Wait()
}

func ExampleFromCmd_trytInterupt() {
	cmd := exec.Command("sleep", "99999")
	ctx, cancel := context.WithCancel(context.Background())

	c := FromCmd(ctx, cmd, func() {
		done := make(chan struct{})

		go func() {
			cmd.Wait()
			close(done)
		}()

		cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
	})

	if err := c.Start(); err != nil {
		panic(err)
	}

	cancel()
	c.Wait()
}
