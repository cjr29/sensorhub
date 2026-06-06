package main

/*
	This package creats a custom slog handler without the Keys
	Only the values are displayed.
	It uses Mutex to avoid race and blocking issues on threads.

	Supports log.Info, log.Debug, log.Warn, log.Error, etc. with the same syntax as the standard library slog.
*/

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type SlogHandler struct {
	h   slog.Handler
	mu  *sync.Mutex
	out io.Writer
}

func ConfigureLogging(logLevel slog.Level, logging bool) *slog.Logger {
	var programLevel = new(slog.LevelVar)
	programLevel.Set(logLevel)
	// Send to stdout, could be a file or other io.Writer
	log := slog.New(SlogNewHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel, AddSource: false}))
	if logging {
		log.Debug("Log handler configured.")
	}
	return log
}

func SlogNewHandler(o io.Writer, opts *slog.HandlerOptions) *SlogHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &SlogHandler{
		out: o,
		h: slog.NewTextHandler(o, &slog.HandlerOptions{
			Level:       opts.Level,
			AddSource:   opts.AddSource,
			ReplaceAttr: nil,
		}),
		mu: &sync.Mutex{},
	}
}

func (h *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{h: h.h.WithAttrs(attrs), out: h.out, mu: h.mu}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{h: h.h.WithGroup(name), out: h.out, mu: h.mu}
}

func (h *SlogHandler) Handle(ctx context.Context, r slog.Record) error {

	formattedTime := r.Time.Format("2006/01/02 15:04:05")

	//add time and message to values
	strs := []string{formattedTime, r.Message, "\n"}

	if r.NumAttrs() != 0 {
		r.Attrs(func(a slog.Attr) bool {
			strs = append(strs, a.Value.String())
			return true
		})
	}

	result := strings.Join(strs, " ")
	b := []byte(result)

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.out.Write(b)

	return err

}
