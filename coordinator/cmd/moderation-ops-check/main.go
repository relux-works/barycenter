package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"relux.works/duet/coordinator/internal/moderationops"
)

func main() {
	configPath := flag.String("config", "../docs/compliance/moderation-operations.json", "operations contract path")
	runbookPath := flag.String("runbook", "../docs/moderation-operations-runbook.md", "runbook path")
	requireMailReady := flag.Bool("require-mail-ready", false, "require ready state and live MX records")
	flag.Parse()
	operations, err := moderationops.Load(*configPath)
	if err == nil {
		err = moderationops.ValidateRunbook(*runbookPath)
	}
	if err == nil && *requireMailReady {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = moderationops.VerifyReadyMail(ctx, net.DefaultResolver, operations)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("moderation operations valid; mail_delivery=%s\n", operations.Mail.DeliveryState)
}
