package obs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

func Setup(level, exporter string, logOut, traceOut io.Writer) (func(context.Context) error, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("obs: log level %q: %w", level, err)
	}
	SetupLogging(logOut, lvl)

	switch exporter {
	case "console":
		return SetupTracing(traceOut)
	case "none", "":
		return SetupTracing(nil)
	}
	return nil, fmt.Errorf("obs: unknown trace exporter %q", exporter)
}
