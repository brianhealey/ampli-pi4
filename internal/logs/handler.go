package logs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// AppSource is the source id used for entries captured from the daemon's own
// structured logger.
const AppSource = "app"

// Handler is a slog.Handler that mirrors every record into a RingBuffer before
// delegating to an inner handler. This makes the daemon's own log readable over
// the API without depending on systemd, which is unavailable in mock/dev runs.
type Handler struct {
	inner  slog.Handler
	buf    *RingBuffer
	attrs  []slog.Attr // already qualified with their group path
	groups []string
}

// NewHandler wraps inner so that records are also captured into buf.
func NewHandler(inner slog.Handler, buf *RingBuffer) *Handler {
	return &Handler{inner: inner, buf: buf}
}

// Enabled defers to the inner handler's level filtering.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle captures the record then forwards it to the inner handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	h.buf.Append(Entry{
		Time:    r.Time,
		Level:   levelFromSlog(r.Level),
		Message: h.render(r),
		Source:  AppSource,
	})
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a handler with additional pre-bound attributes.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.clone()
	next.inner = h.inner.WithAttrs(attrs)
	for _, a := range attrs {
		next.attrs = append(next.attrs, qualify(h.groups, a))
	}
	return next
}

// WithGroup returns a handler that nests subsequent attributes under name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := h.clone()
	next.inner = h.inner.WithGroup(name)
	next.groups = append(next.groups, name)
	return next
}

// clone copies the handler, detaching the slices so appends by one derived
// handler cannot be observed by a sibling.
func (h *Handler) clone() *Handler {
	return &Handler{
		inner:  h.inner,
		buf:    h.buf,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: append([]string(nil), h.groups...),
	}
}

// render flattens a record into "message key=value ..." form for display.
func (h *Handler) render(r slog.Record) string {
	var sb strings.Builder
	sb.WriteString(r.Message)

	for _, a := range h.attrs {
		writeAttr(&sb, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&sb, qualify(h.groups, a))
		return true
	})
	return sb.String()
}

// writeAttr appends " key=value" for a single attribute, skipping empty ones.
func writeAttr(sb *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	fmt.Fprintf(sb, " %s=%v", a.Key, a.Value.Resolve().Any())
}

// qualify prefixes an attribute key with its enclosing group path.
func qualify(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 {
		return a
	}
	a.Key = strings.Join(groups, ".") + "." + a.Key
	return a
}

// levelFromSlog maps slog levels onto the package's normalised severities.
func levelFromSlog(l slog.Level) Level {
	switch {
	case l >= slog.LevelError:
		return LevelError
	case l >= slog.LevelWarn:
		return LevelWarn
	case l >= slog.LevelInfo:
		return LevelInfo
	default:
		return LevelDebug
	}
}
