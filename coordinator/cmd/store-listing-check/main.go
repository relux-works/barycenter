package main

import (
	"flag"
	"fmt"
	"log"

	"relux.works/duet/coordinator/internal/storelisting"
)

func main() {
	log.SetFlags(0)
	packPath := flag.String("pack", "../docs/store/phase1/partner-center-package.json", "Partner Center package manifest")
	repoRoot := flag.String("repo-root", "..", "repository root for package resources")
	requireReady := flag.Bool("require-ready", false, "require real screenshots, WACK, IARC, exact build and owner proceed")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("usage: store-listing-check [--pack path] [--repo-root path] [--require-ready]")
	}
	pack, err := storelisting.Load(*packPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := pack.Validate(*repoRoot, *requireReady); err != nil {
		log.Fatal(err)
	}
	if *requireReady {
		fmt.Println("Partner Center package is frozen and submission-ready")
		return
	}
	fmt.Println("Partner Center package engineering shape is valid; manual screenshots, WACK, IARC and exact-build owner proceed remain required")
}
