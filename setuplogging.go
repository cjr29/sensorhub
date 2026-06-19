package main

/*
	This package creats a custom slog handler without the Keys
	Only the values are displayed.
	It uses Mutex to avoid race and blocking issues on threads.

	Supports log.Info, log.Debug, log.Warn, log.Error, etc. with the same syntax as the standard library slog.
*/

/*
type SlogHandler struct {
	h    slog.Handler
	mu   *sync.Mutex
	file *os.File
}

func ConfigureLogging(logLevel slog.Level, logging bool) *slog.Logger {
	// Ensure the log directory exists before attempting to create the log file.
	if err := ensureLogDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log dir: %v\n", err)
		os.Exit(1)
	}
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	log := slog.New(SlogNewHandler(f, &slog.HandlerOptions{Level: logLevel, AddSource: false}))
	slog.SetDefault(log)
	if logging {
		log.Info(fmt.Sprintf("Log handler configured for %s.", logLevel.String()))
	}
	return log
}

func SlogNewHandler(o *os.File, opts *slog.HandlerOptions) *SlogHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &SlogHandler{
		file: o,
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
	return &SlogHandler{h: h.h.WithAttrs(attrs), file: h.file, mu: h.mu}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{h: h.h.WithGroup(name), file: h.file, mu: h.mu}
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

	_, err := h.file.Write(b)

	return err
}

// Reopen closes the current file and opens a new one at the same path, swapping the handler.
func (sh *SlogHandler) Reopen(out io.Writer) error {
	newf, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	newh := slog.NewTextHandler(newf, &slog.HandlerOptions{
		Level:       LogLevel,
		AddSource:   false,
		ReplaceAttr: nil,
	})
	log := slog.New(SlogNewHandler(newf, &slog.HandlerOptions{Level: LogLevel, AddSource: false}))
	slog.SetDefault(log)

	sh.mu.Lock()
	oldf := sh.file
	sh.file = newf
	sh.h = newh
	sh.mu.Unlock()
	if oldf != nil {
		_ = oldf.Close()
	}
	return nil
}
*/
