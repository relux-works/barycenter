package media

import (
	"context"
	"errors"
)

var errCommandOutputLimit = errors.New("worker output exceeded its capture limit")

type commandSpec struct {
	Tool        string
	Args        []string
	StdoutLimit int64
	StderrLimit int64
	CPUSeconds  uint64
	MemoryBytes uint64
	OpenFiles   uint64
	FileBytes   uint64
}

type commandResult struct {
	Stdout []byte
	Stderr []byte
}

type commandRunner interface {
	Run(context.Context, commandSpec) (commandResult, error)
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, spec commandSpec) (commandResult, error) {
	return runLimitedCommand(ctx, spec)
}

type cappedCapture struct {
	limit    int64
	data     []byte
	exceeded bool
}

func (capture *cappedCapture) Write(value []byte) (int, error) {
	length := len(value)
	remaining := capture.limit - int64(len(capture.data))
	if remaining > 0 {
		keep := int64(len(value))
		if keep > remaining {
			keep = remaining
		}
		capture.data = append(capture.data, value[:int(keep)]...)
	}
	if int64(length) > remaining {
		capture.exceeded = true
	}
	return length, nil
}

type tailCapture struct {
	limit int64
	data  []byte
}

func (capture *tailCapture) Write(value []byte) (int, error) {
	length := len(value)
	if capture.limit <= 0 {
		return length, nil
	}
	if int64(len(value)) >= capture.limit {
		capture.data = append(capture.data[:0], value[len(value)-int(capture.limit):]...)
		return length, nil
	}
	capture.data = append(capture.data, value...)
	if int64(len(capture.data)) > capture.limit {
		capture.data = append(capture.data[:0], capture.data[len(capture.data)-int(capture.limit):]...)
	}
	return length, nil
}
