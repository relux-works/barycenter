package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"relux.works/duet/coordinator/internal/legalops"
	"relux.works/duet/coordinator/internal/policypack"
	"relux.works/duet/coordinator/internal/policypublication"
)

func main() {
	log.SetFlags(0)
	repoRoot := flag.String("repo-root", "..", "barycenter repository root")
	packPath := flag.String("pack", "../docs/compliance/policy-pack-2026-07-14.json", "policy-pack manifest")
	inputsPath := flag.String("inputs", "../docs/compliance/legal-ops-inputs.json", "approved input manifest")
	origin := flag.String("origin", policypublication.ProductionOrigin, "public HTTPS origin")
	requireProceed := flag.Bool("require-proceed", false, "require exact source approval")
	live := flag.Bool("live", false, "verify public pages, hashes and cache headers")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("usage: policy-site-check [--live] [--require-proceed] [--origin URL]")
	}
	pack, err := policypack.Load(*packPath)
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
	if !*live {
		fmt.Fprintf(os.Stdout, "policy publication source valid; decision=%s\n", pack.Review.PublicationDecision)
		return
	}
	packBytes, err := os.ReadFile(*packPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := policypublication.VerifyLive(context.Background(), policypublication.DefaultHTTPClient(), *origin, packBytes, pack); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "public policy pages match approved source hashes and cache contract")
}
