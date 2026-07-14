package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"relux.works/duet/coordinator/internal/legalops"
	"relux.works/duet/coordinator/internal/policypack"
)

func main() {
	log.SetFlags(0)
	manifestPath := flag.String("manifest", "../docs/compliance/policy-pack-2026-07-14.json", "policy-pack manifest path")
	inputsPath := flag.String("inputs", "../docs/compliance/legal-ops-inputs.json", "approved legal/operations input path")
	repoRoot := flag.String("repo-root", "..", "repository root used to resolve manifest document paths")
	requireProceed := flag.Bool("require-proceed", false, "fail unless the configured owner approved the exact document hashes")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("usage: policy-pack-check [--manifest path] [--inputs path] [--repo-root path] [--require-proceed]")
	}

	pack, err := policypack.Load(*manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	inputs, err := legalops.Load(*inputsPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := pack.Validate(*repoRoot, inputs, *requireProceed); err != nil {
		log.Fatal(err)
	}
	if pack.Review.PublicationDecision == "hold" {
		fmt.Fprintln(os.Stdout, "policy pack valid; exact-content publication decision=hold")
		return
	}
	fmt.Fprintln(os.Stdout, "policy pack valid and exact document hashes approved for publication")
}
