package exitcode

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const (
	ansiRed   = "\x1b[31m"
	ansiBold  = "\x1b[1m"
	ansiReset = "\x1b[0m"
)

// Report maps a CLI Run error to a process exit code
func Report(ctx context.Context, err error, prefix string) int {
	if err == nil {
		return OK
	}
	var exitErr cli.ExitCoder
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if ctx.Err() != nil {
		return Interrupted
	}
	if term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("NO_COLOR") == "" {
		fmt.Fprintf(os.Stderr, "%s%s%s:%s %v\n", ansiBold, ansiRed, prefix, ansiReset, err)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prefix, err)
	}
	return Error
}
