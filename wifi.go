package main

import (
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"strings"
)

// WiFiNetwork holds the parsed data for each Wi-Fi access point
type WiFiNetwork struct {
	SSID     string
	BSSID    string
	Security string
	Signal   string
	Bars     string
}

// HTML Template for rendering the Wi-Fi list as a clean table
const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
	<title>Available Wi-Fi Networks</title>
	<style>
		body { font-family: sans-serif; margin: 40px; }
		table { border-collapse: collapse; width: 100%; max-width: 800px; }
		th, td { border: 1px solid #dddddd; padding: 10px; text-align: left; }
		th { background-color: #f2f2f2; }
	</style>
</head>
<body>
	<h2>Available Wi-Fi Networks</h2>
	<table>
		<thead>
			<tr>
				<th>SSID</th>
				<th>BSSID</th>
				<th>Security</th>
				<th>Signal</th>
				<th>Bars</th>
			</tr>
		</thead>
		<tbody>
			{{range .}}
			<tr>
				<td>{{.SSID}}</td>
				<td>{{.BSSID}}</td>
				<td>{{.Security}}</td>
				<td>{{.Signal}}</td>
				<td>{{.Bars}}</td>
			</tr>
			{{end}}
		</tbody>
	</table>
</body>
</html>`

func getAvailableNetworksHTML() ([]WiFiNetwork, error) {
	// Execute nmcli to get Wi-Fi list, parse columns, skipping the header line
	// Note: We use grep to hide the asterisk (*) that marks the active network
	cmd := exec.Command("sh", "-c", "nmcli -t -f SSID,BSSID,SECURITY,SIGNAL,BARS dev wifi list | grep -v '^\\*'")
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Error executing nmcli: %s\n", err)
		return nil, err
	}

	// Parse the nmcli output (fields are colon-separated since we used -t)
	var networks []WiFiNetwork
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) == 5 {
			networks = append(networks, WiFiNetwork{
				SSID:     parts[0],
				BSSID:    parts[1],
				Security: parts[2],
				Signal:   parts[3],
				Bars:     parts[4],
			})
		}
	}

	// Parse and execute the HTML template
	tmpl, err := template.New("wifiTable").Parse(htmlTemplate)
	if err != nil {
		fmt.Printf("Error parsing template: %s\n", err)
		return nil, err
	}

	// Output to stdout (or you can use http.ResponseWriter in a web server)
	err = tmpl.Execute(os.Stdout, networks)
	if err != nil {
		fmt.Printf("Error executing template: %s\n", err)
		return nil, err
	}

	return networks, nil
}

func getAvailableNetworks() ([]byte, error) {
	// Request NetworkManager to connect to target network
	// nmcli automatically handles turning off the current AP when joining a client network
	cmd := exec.Command("nmcli", "device", "wifi", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list Wi-Fi networks: %w", err)
	}
	return output, nil
}

// func getAvailableNetworks() ([]WiFiNetwork, error) {
// 	cmd := exec.Command("nmcli", "-t", "-f", "SSID,BSSID,SECURITY,SIGNAL,BARS", "device", "wifi", "list")
// 	output, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list Wi-Fi networks: %w", err)
// 	}

// 	lines := string(output)
// 	var networks []WiFiNetwork
// 	for _, line := range splitLines(lines) {
// 		fields := splitFields(line)
// 		if len(fields) >= 5 {
// 			networks = append(networks, WiFiNetwork{
// 				SSID:     fields[0],
// 				BSSID:    fields[1],
// 				Security: fields[2],
// 				Signal:   fields[3],
// 				Bars:     fields[4],
// 			})
// 		}
// 	}
// 	return networks, nil
// }

// func splitLines(s string) []string {
// 	var lines []string
// 	current := ""
// 	for _, r := range s {
// 		if r == '\n' {
// 			lines = append(lines, current)
// 			current = ""
// 		} else {
// 			current += string(r)
// 		}
// 	}
// 	if current != "" {
// 		lines = append(lines, current)
// 	}
// 	return lines
// }

// func splitFields(s string) []string {
// 	var fields []string
// 	current := ""
// 	for _, r := range s {
// 		if r == ':' {
// 			fields = append(fields, current)
// 			current = ""
// 		} else {
// 			current += string(r)
// 		}
// 	}
// 	fields = append(fields, current)
// 	return fields
// }
