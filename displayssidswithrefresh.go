package main

import (
	"bytes"
	"html/template"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Configuration
const (
	listenAddr     = ":8080"
	refreshSeconds = 5
	nmcliTimeout   = 5 * time.Second
)

// Template for the HTML page
var pageTmpl = template.Must(template.New("page").Parse(`
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Available Wi‑Fi Networks</title>
<style>
  body { font-family: system-ui, -apple-system, Roboto, "Segoe UI", Arial; padding: 1rem; }
  table { border-collapse: collapse; width: 100%; max-width: 900px; }
  th, td { text-align: left; padding: .5rem; border-bottom: 1px solid #e1e1e1; }
  th { background: #f7f7f7; }
  .muted { color: #666; font-size: .9rem; }
</style>
<meta http-equiv="refresh" content="{{.Refresh}}">
</head>
<body>
<a href="/">← Back</a>
<h1>Available Wi‑Fi Networks</h1>
<p class="muted">Auto-refresh every {{.Refresh}} seconds</p>
{{if .Error}}
  <p style="color:red">Error: {{.Error}}</p>
{{end}}
{{if .Networks}}
<table>
  <thead><tr><th>SSID</th><th>SIGNAL</th><th>SECURITY</th></tr></thead>
  <tbody>
  {{range .Networks}}
    <tr>
      <td>{{.SSID}}</td>
      <td>{{.BSSID}}</td>
      <td>{{.CHAN}}</td>
      <td>{{.RATE}}</td>
      <td>{{.SIGNAL}}</td>
      <td>{{.SECURITY}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
{{else}}
  <p>No networks found.</p>
{{end}}
</body>
</html>`))

type Network struct {
	SSID     string
	BSSID    string
	CHAN     string
	RATE     string
	SIGNAL   string
	SECURITY string
}

type SSIDPageData struct {
	Refresh  int
	Networks []Network
	Error    string
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
			SSID: parts[0],
			// BSSID:    parts[1],
			// CHAN:     parts[2],
			// RATE:     parts[3],
			SIGNAL:   parts[1],
			SECURITY: parts[2],
		})
	}
	return nets
}

func handler(w http.ResponseWriter, r *http.Request) {
	data := SSIDPageData{Refresh: refreshSeconds}

	// Use nmcli to list Wi-Fi networks in terse machine-friendly format
	// fields: SSID,BSSID,CHAN,RATE,SIGNAL,SECURITY
	out, err := runNmcli("-p", "-f", "SSID,SIGNAL,SECURITY", "device", "wifi", "list")
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

// NOT USED IN MAIN FLOW, but can be used for quick testing of the page template and nmcli parsing logic
// func main() {
// 	http.HandleFunc("/", handler)
// 	log.Printf("listening on %s (refresh %d s)\n", listenAddr, refreshSeconds)
// 	if err := http.ListenAndServe(listenAddr, nil); err != nil {
// 		log.Fatal(err)
// 	}
// }
