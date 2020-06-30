# execctx
Wrap Go's os/exec with custom context handling

Go's `os/exec` package has a `CommandContext` package, which allows you to create a command that can be cancelled by context.
The downside of using `exec.CommandContext` is the handling of the cancelled context cannot be coustomized. As such a cancelled
context will always send `SIGKILL to the command.

This package allows you to add a custom handler.

Example Usage:

```
ctx, cancel := context.WithCancel(context.Background)
defer cancel()

cmd := exec.Command("sleep", "99999")
eCmd := execctx.FromCmd(ctx, cmd, func() {
  // some custom things and then....
  cmd.Process.Kill()
})

eCmd.Start()
```

In the handler you may want to do something fancy, like send SIGINT/SIGTERM, wait for a period of time, and then send SIGKILL.
Note that is is completely up to you to ensure that this actually causes the process to exit.
