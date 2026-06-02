package exitcode

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const (
	ansiRed   = "\x1b[31m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

const (
	styleError  = ansiBold + ansiRed
	styleNotice = ansiDim
)

func emit(style, label, msg string) {
	if term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("NO_COLOR") == "" {
		fmt.Fprintf(os.Stderr, "%s%s:%s %v\n", style, label, ansiReset, msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %v\n", label, msg)
	}
}

// Notice prints informational message.
func Notice(msg string) {
	emit(styleNotice, gotext.Get("Notice"), msg)
}

// Report maps a CLI Run error to a process exit code, printing it when relevant.
func Report(ctx context.Context, err error) int {
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
	emit(styleError, gotext.Get("Error"), err.Error())
	return Error
}
