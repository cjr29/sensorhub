package main

import (
	"log"
	"net/http"
	"strings"
)

func configureViaHotspot() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/page1", listHandler)
	http.HandleFunc("/page2", loginHandler)

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	log.Println("Listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Println("template execute error:", err)
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
	// fields: SSID,BSSID,CHAN,RATE,SIGNAL,SECURITY
	out, err := runNmcli("-t", "-f", "SSID,BSSID,CHAN,RATE,SIGNAL,SECURITY", "device", "wifi", "list")
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
