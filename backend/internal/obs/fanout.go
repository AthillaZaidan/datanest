package obs

import (
	"context"
	"errors"
	"log/slog"
)

type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

type leveled struct {
	slog.Handler
	level slog.Leveler
}

func (h leveled) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level.Level() && h.Handler.Enabled(ctx, level)
}

func (h leveled) WithAttrs(attrs []slog.Attr) slog.Handler {
	return leveled{h.Handler.WithAttrs(attrs), h.level}
}

func (h leveled) WithGroup(name string) slog.Handler {
	return leveled{h.Handler.WithGroup(name), h.level}
}

type masking struct{ inner slog.Handler }

func (h masking) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h masking) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(maskTree(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h masking) WithAttrs(attrs []slog.Attr) slog.Handler {
	masked := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		masked[i] = maskTree(a)
	}
	return masking{h.inner.WithAttrs(masked)}
}

func (h masking) WithGroup(name string) slog.Handler { return masking{h.inner.WithGroup(name)} }

func maskTree(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() != slog.KindGroup {
		return MaskAttr(a)
	}
	members := a.Value.Group()
	masked := make([]any, len(members))
	for i, m := range members {
		masked[i] = maskTree(m)
	}
	return slog.Group(a.Key, masked...)
}

type correlationHandler struct{ slog.Handler }

func (h correlationHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := CorrelationID(ctx); id != "" {
		r.AddAttrs(slog.String(AttrCorrelationID, id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h correlationHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return correlationHandler{h.Handler.WithAttrs(attrs)}
}

func (h correlationHandler) WithGroup(name string) slog.Handler {
	return correlationHandler{h.Handler.WithGroup(name)}
}
