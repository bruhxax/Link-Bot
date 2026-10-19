//go:build !linux

package updater

import (
	"os/exec"
	"time"
)

func configureCommand(cmd *exec.Cmd) { cmd.WaitDelay = 5 * time.Second }
