//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("pulsar-win-probe is a Windows-only packaged AppContainer probe")
}
