package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

var tpl = template.Must(template.New("menu").Parse(`
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Menu</title>
  <style>
    body { font-family: system-ui, -apple-system, "Segoe UI", Roboto, Arial; padding: 2rem; }
    .menu { max-width: 420px; margin: 0 auto; }
    .choice { margin: .5rem 0; }
    button { padding: .5rem 1rem; }
  </style>
</head>
<body>
  <div class="menu">
    <h1>Choose an option</h1>
    <form method="POST" action="/choose">
      {{range .}}
        <div class="choice">
          <label>
            <input type="radio" name="choice" value="{{.Value}}" required>
            {{.Label}}
          </label>
        </div>
      {{end}}
      <button type="submit">Submit</button>
    </form>
  </div>
</body>
</html>
`))

type MenuItem struct {
	Label string
	Value string
}

func menuhtml() {
	menu := []MenuItem{
		{"Start new session", "start"},
		{"List Available Networks", "list"},
		{"Settings", "settings"},
		{"Help", "help"},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tpl.Execute(w, menu); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			log.Println("template execute:", err)
		}
	})

	http.HandleFunc("/choose", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		choice := r.FormValue("choice")
		switch choice {
		case "start":
			fmt.Fprintln(w, "You selected: Start new session")
		case "list":
			fmt.Fprintln(w, "You selected: List available networks")
			displaySSIDsWithRefresh()
		case "settings":
			fmt.Fprintln(w, "You selected: Settings")
		case "help":
			fmt.Fprintln(w, "You selected: Help")
		default:
			http.Error(w, "unknown choice", http.StatusBadRequest)
		}
	})

	addr := ":8080"
	log.Printf("listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
