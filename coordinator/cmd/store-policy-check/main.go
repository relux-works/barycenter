package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"relux.works/duet/coordinator/internal/storepolicy"
)

func main() {
	baselinePath := flag.String("baseline", "../docs/compliance/store-policy-baseline-2026-07-14.json", "path to the checked-in Store policy baseline")
	recordPath := flag.String("record", "../docs/compliance/store-policy-pre-submit.json", "path to the checked-in pre-submit delta record")
	requireProceed := flag.Bool("require-proceed", false, "require a fresh proceed decision for an external submission")
	maxAge := flag.Duration("max-age", 24*time.Hour, "maximum age of a proceed verification")
	tag := flag.String("tag", "", "release tag that the proceed record must authorize")
	flag.Parse()

	baseline, err := storepolicy.LoadBaseline(*baselinePath)
	if err != nil {
		fail(err)
	}
	record, err := storepolicy.LoadPreSubmitRecord(*recordPath)
	if err != nil {
		fail(err)
	}
	if *requireProceed && *tag == "" {
		fail(fmt.Errorf("--tag is required with --require-proceed"))
	}
	if err := record.Validate(baseline, time.Now(), *maxAge, *tag, *requireProceed); err != nil {
		fail(err)
	}
	fmt.Printf("Store policy gate valid: baseline=%s policy=%s decision=%s verified_at=%s\n",
		baseline.SnapshotDate, record.PolicyVersion, record.Decision, record.VerifiedAt)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Store policy gate failed:", err)
	os.Exit(1)
}
