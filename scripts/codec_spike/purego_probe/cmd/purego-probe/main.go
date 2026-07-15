package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	probe "live.barycenter/purego-codec-probe"
)

func main() {
	fixtures := flag.String("fixtures", "", "path to the frozen smoke fixture directory")
	output := flag.String("output", "", "evidence JSON output")
	flag.Parse()
	if *fixtures == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "-fixtures and -output are required")
		os.Exit(2)
	}
	encoded, err := json.MarshalIndent(probe.Run(*fixtures), "", "  ")
	if err != nil {
		panic(err)
	}
	encoded = append(encoded, '\n')
	if err = os.WriteFile(*output, encoded, 0o600); err != nil {
		panic(err)
	}
}
