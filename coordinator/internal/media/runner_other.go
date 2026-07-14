//go:build !linux

package media

import (
	"context"
	"errors"
	"os/exec"
)

// Non-Linux builds are development/test clients, not the production
// coordinator image. They retain deadline, fixed-argument, protocol and
// output caps; the production Linux runner additionally applies kernel
// CPU/address-space/file-size/open-file rlimits before exec.
func runLimitedCommand(ctx context.Context, spec commandSpec) (commandResult, error) {
	if ctx == nil || spec.Tool == "" || spec.StdoutLimit <= 0 || spec.StderrLimit <= 0 {
		return commandResult{}, errors.New("invalid worker command")
	}
	command := exec.CommandContext(ctx, spec.Tool, spec.Args...)
	command.Env = []string{"LANG=C", "LC_ALL=C", "HOME=/nonexistent", "PATH=/usr/bin:/bin"}
	stdout := &cappedCapture{limit: spec.StdoutLimit}
	stderr := &tailCapture{limit: spec.StderrLimit}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	result := commandResult{Stdout: stdout.data, Stderr: stderr.data}
	if stdout.exceeded {
		return result, errCommandOutputLimit
	}
	if err != nil {
		return result, errors.New("media worker exited unsuccessfully")
	}
	return result, nil
}
