package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"relux.works/duet/coordinator/internal/legalops"
	"relux.works/duet/coordinator/internal/policypack"
	"relux.works/duet/coordinator/internal/policypublication"
)

func main() {
	log.SetFlags(0)
	repoRoot := flag.String("repo-root", "..", "barycenter repository root")
	packPath := flag.String("pack", "../docs/compliance/policy-pack-2026-07-14.json", "policy-pack manifest")
	inputsPath := flag.String("inputs", "../docs/compliance/legal-ops-inputs.json", "approved input manifest")
	output := flag.String("output", "", "pulsar-site checkout root")
	upstreamCommit := flag.String("upstream-commit", "", "exact barycenter source commit")
	check := flag.Bool("check", false, "compare the site checkout with deterministic output")
	requireProceed := flag.Bool("require-proceed", false, "fail unless exact source hashes are approved")
	flag.Parse()
	if flag.NArg() != 0 || *output == "" || *upstreamCommit == "" {
		log.Fatal("usage: policy-site-build --output path --upstream-commit sha [--check] [--require-proceed]")
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
	files, manifest, err := policypublication.Build(*repoRoot, *packPath, *upstreamCommit, pack)
	if err != nil {
		log.Fatal(err)
	}
	if *check {
		err = policypublication.Check(*output, files)
	} else {
		err = policypublication.Write(*output, files)
	}
	if err != nil {
		log.Fatal(err)
	}
	action := "wrote"
	if *check {
		action = "verified"
	}
	abs, _ := filepath.Abs(*output)
	fmt.Printf("%s %d policy-site files at %s; deployment_state=%s\n", action, len(files), abs, manifest.DeploymentState)
}
