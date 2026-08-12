package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/lov3g00d/ctxscope"
)

func main() {
	if err := writeReport(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func writeReport(output io.Writer) error {
	report, err := ctxscope.InspectScoped(
		context.Background(),
		func(scope *ctxscope.Scope) {
			scope.Task("queued email", func(context.Context) {})
		},
		ctxscope.WithName("email delivery"),
		ctxscope.WithGrace(10*time.Millisecond),
		ctxscope.WithPollInterval(time.Millisecond),
	)
	if err != nil {
		return fmt.Errorf("inspect email delivery: %w", err)
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}

	return nil
}
