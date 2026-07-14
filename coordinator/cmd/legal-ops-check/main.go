package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"relux.works/duet/coordinator/internal/legalops"
)

func main() {
	log.SetFlags(0)
	requireApproved := flag.Bool("require-approved", false, "fail unless every external input and the publication gate are approved")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: legal-ops-check [--require-approved] <checkpoint.json>")
	}

	checkpoint, err := legalops.Load(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	if err := checkpoint.Validate(*requireApproved); err != nil {
		log.Fatal(err)
	}
	if unresolved := checkpoint.Unresolved(); len(unresolved) > 0 {
		fmt.Fprintf(os.Stdout, "legal/operations checkpoint valid but publication blocked: %v\n", unresolved)
		return
	}
	fmt.Fprintln(os.Stdout, "legal/operations publication gate approved")
}
