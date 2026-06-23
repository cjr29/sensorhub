package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

var tmpl = template.Must(template.New("").ParseGlob("/usr/local/bin/sensorhub/templates/*.html"))
var pageTmpl = template.Must(template.ParseFiles("/usr/local/bin/sensorhub/templates/showwifi.html"))

// var loginTmpl = template.Must(template.ParseFiles("/usr/local/bin/sensorhub/templates/login.html"))

func configureViaHotspot() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/page1", listHandler)
	http.HandleFunc("/page2", loginHandler)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/usr/local/bin/sensorhub/static"))))

	logger.Info(fmt.Sprintf("Sensor Hub HTTP server starting on port %s for WiFi configuration.", listenAddr))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Error(err.Error())
	}
}

func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		logger.Error("template execute error", "error", err.Error())
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "index.html", map[string]any{
		"Title": "Sensor Hub Setup",
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// render(w, "page2.html", map[string]any{
	// 	"Title": "Login to WiFi Network",
	// 	"Body":  "Enter login information for selected WiFi network.",
	// })
	setupViaHotspot()
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	data := SSIDPageData{Refresh: refreshSeconds}

	// Use nmcli to list Wi-Fi networks in terse machine-friendly format
	// fields: SSID,SIGNAL,SECURITY, -p pretty, -t terse, -f fields
	out, err := runNmcli("-t", "-f", "SSID,SIGNAL,SECURITY", "device", "wifi", "list")
	if err != nil {
		data.Error = strings.TrimSpace(err.Error() + " — output: " + out)
		_ = pageTmpl.Execute(w, data)
		return
	}

	data.Networks = parseNmcliScan(out)
	if err := pageTmpl.Execute(w, data); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
}

// runNmcli runs nmcli command with timeout and returns stdout or an error.
func runNmcli(args ...string) (string, error) {
	cmd := exec.Command("nmcli", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(nmcliTimeout):
		_ = cmd.Process.Kill()
		return "", contextError("nmcli timed out")
	}
}

func contextError(msg string) error {
	return &customError{msg}
}

type customError struct{ s string }

func (e *customError) Error() string { return e.s }

// parseNmcliScan parses nmcli --terse --fields ... output (colon separated)
func parseNmcliScan(raw string) []Network {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var nets []Network
	for _, l := range lines {
		if l == "" {
			continue
		}
		parts := strings.Split(l, ":")
		// normalize length
		for len(parts) < 6 {
			parts = append(parts, "")
		}
		nets = append(nets, Network{
			SSID:     parts[0],
			SIGNAL:   parts[1],
			SECURITY: parts[2],
		})
	}
	return nets
}
