package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const htmlTmpl = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>Available SSIDs</title>
<style>body{font-family:system-ui,Segoe UI,Roboto,Helvetica,Arial;margin:20px}ul{list-style:none;padding:0}li{padding:6px 0;border-bottom:1px solid #eee}</style>
</head>
<body>
<h1>Available SSIDs</h1>
<p>Scanned: {{.When}}</p>
<ul>
{{range .SSIDs}}<li>{{.}}</li>{{else}}<li><em>No networks found</em></li>{{end}}
</ul>
</body>
</html>`

var tmpl = template.Must(template.New("page").Parse(htmlTmpl))

type PageData struct {
	When  string
	SSIDs []string
}

func scanSSIDs() ([]string, error) {
	// Use nmcli to list SSIDs (name col, skip header). Adjust if nmcli isn't available.
	// `nmcli -t -f SSID dev wifi` gives one SSID per line (may contain duplicates/empty).
	cmd := exec.Command("nmcli", "-t", "-f", "SSID", "device", "wifi", "list")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	lines := strings.Split(out.String(), "\n")
	seen := make(map[string]struct{})
	var ssids []string
	for _, l := range lines {
		s := strings.TrimSpace(l)
		if s == "" {
			continue
		}
		// nmcli may include colons for other fields in some locales; we used -f SSID so typically it's raw SSID.
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			ssids = append(ssids, s)
		}
	}
	return ssids, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	ssids, err := scanSSIDs()
	if err != nil {
		http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data := PageData{
		When:  time.Now().Format(time.RFC1123),
		SSIDs: ssids,
	}
	if err := tmpl.Execute(w, data); err != nil {
		log.Println("template error:", err)
	}
}

// func main() {
// 	http.HandleFunc("/", handler)
// 	addr := ":8080"
// 	log.Println("listening on", addr)
// 	if err := http.ListenAndServe(addr, nil); err != nil {
// 		log.Fatal(err)
// 	}
// }

func displaySSIDs() {
	http.HandleFunc("/", handler)
	addr := ":8080"
	log.Println("listening on", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
