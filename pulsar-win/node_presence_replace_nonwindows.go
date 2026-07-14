//go:build !windows

package main

import "os"

func replaceStateFile(from, to string) error { return os.Rename(from, to) }
