package main

import (
	"html/template"
	"log"
	"net/http"
	"strings"
)

var tmpl = template.Must(template.New("").ParseGlob("/usr/local/bin/sensorhub/templates/*.html"))
var pageTmpl = template.Must(template.ParseFiles("/usr/local/bin/sensorhub/templates/showwifi.html"))

func configureViaHotspot() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/page1", listHandler)
	http.HandleFunc("/page2", loginHandler)

	// http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/usr/local/bin/sensorhub/static"))))

	log.Println("Sensor Hub listening on :8080, for WiFi configuration...")
	logger.Info("Sensor Hub HTTP server starting", "address", listenAddr)
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

// func page1Handler(w http.ResponseWriter, r *http.Request) {
// 	render(w, "page1.html", map[string]any{
// 		"Title": "List of Available Networks",
// 		"Body":  "Displays available WiFi networks, refreshed every 5 seconds.",
// 	})
// }

func loginHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "page2.html", map[string]any{
		"Title": "Login to WiFi Network",
		"Body":  "Enter login information for selected WiFi network.",
	})
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
