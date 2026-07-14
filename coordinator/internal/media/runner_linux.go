//go:build linux

package media

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

const limitedWorkerScript = `IFS= read -r media_worker_ready <&3 || exit 125
exec 3<&-
exec "$@"`

func runLimitedCommand(ctx context.Context, spec commandSpec) (commandResult, error) {
	if ctx == nil || spec.Tool == "" || spec.StdoutLimit <= 0 || spec.StderrLimit <= 0 {
		return commandResult{}, errors.New("invalid worker command")
	}
	releaseRead, releaseWrite, err := os.Pipe()
	if err != nil {
		return commandResult{}, errors.New("create worker limit barrier")
	}
	defer releaseWrite.Close()
	arguments := append([]string{"-c", limitedWorkerScript, "media-worker", spec.Tool}, spec.Args...)
	command := exec.CommandContext(ctx, "/bin/sh", arguments...)
	command.Env = []string{"LANG=C", "LC_ALL=C", "HOME=/nonexistent", "PATH=/usr/bin:/bin"}
	command.ExtraFiles = []*os.File{releaseRead}
	stdout := &cappedCapture{limit: spec.StdoutLimit}
	stderr := &tailCapture{limit: spec.StderrLimit}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		releaseRead.Close()
		return commandResult{}, errors.New("start media worker")
	}
	releaseRead.Close()
	killAndWait := func() {
		_ = command.Process.Kill()
		_, _ = releaseWrite.Write([]byte("stop\n"))
		_ = releaseWrite.Close()
		_ = command.Wait()
	}
	limits := []struct {
		resource int
		value    uint64
	}{
		{unix.RLIMIT_CPU, spec.CPUSeconds},
		{unix.RLIMIT_AS, spec.MemoryBytes},
		{unix.RLIMIT_NOFILE, spec.OpenFiles},
		{unix.RLIMIT_FSIZE, spec.FileBytes},
	}
	for _, limit := range limits {
		if limit.value == 0 {
			continue
		}
		if err := lowerProcessLimit(command.Process.Pid, limit.resource, limit.value); err != nil {
			killAndWait()
			return commandResult{}, errors.New("apply media worker resource limit")
		}
	}
	if _, err := releaseWrite.Write([]byte("ready\n")); err != nil {
		killAndWait()
		return commandResult{}, errors.New("release media worker limit barrier")
	}
	_ = releaseWrite.Close()
	err = command.Wait()
	result := commandResult{Stdout: stdout.data, Stderr: stderr.data}
	if stdout.exceeded {
		return result, errCommandOutputLimit
	}
	if err != nil {
		return result, errors.New("media worker exited unsuccessfully")
	}
	return result, nil
}

func lowerProcessLimit(pid, resource int, requested uint64) error {
	var current unix.Rlimit
	if err := unix.Prlimit(pid, resource, nil, &current); err != nil {
		return err
	}
	if requested > current.Max {
		requested = current.Max
	}
	if requested == 0 {
		return errors.New("media worker resource limit is unavailable")
	}
	return unix.Prlimit(pid, resource, &unix.Rlimit{Cur: requested, Max: current.Max}, nil)
}
