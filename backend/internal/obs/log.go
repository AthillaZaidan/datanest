package obs

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type ctxKey int

const correlationKey ctxKey = 1

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}

func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey).(string)
	return id
}

type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String(AttrTraceID, sc.TraceID().String()), slog.String(AttrSpanID, sc.SpanID().String()))
	}
	if id := CorrelationID(ctx); id != "" {
		r.AddAttrs(slog.String(AttrCorrelationID, id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(name)}
}

func NewHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return ctxHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: ReplaceAttr})}
}

func SetupLogging(w io.Writer, level slog.Leveler) {
	slog.SetDefault(slog.New(NewHandler(w, level)))
}
