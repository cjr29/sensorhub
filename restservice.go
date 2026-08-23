package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"
)

const tableName = "SENSORS"
const restPort = ":5000"

var db *sql.DB

type Sensor struct {
	ID          int     `json:"ID"`
	Time        string  `json:"TIME"`
	SensorID    int     `json:"SENSORID"`
	Model       string  `json:"MODEL"`
	Channel     string  `json:"CHANNEL"`
	Battery     int     `json:"BATTERY"`
	Temperature float64 `json:"TEMPERATURE"`
	Humidity    float64 `json:"HUMIDITY"`
	SensorName  string  `json:"SENSORNAME"`
	Station     string  `json:"STATION"`
	Location    string  `json:"LOCATION"`
	Description string  `json:"DESCRIPTION"`
	Active      int     `json:"ACTIVE"`
}

func restService(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger) {
	defer wg.Done()
	esc := url.PathEscape(dbPath)
	dsn := fmt.Sprintf("file:%s?mode=rw", esc)
	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to open sqlite database: %v", err))
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		logger.Error(fmt.Sprintf("Unable to ping sqlite database: %v", err))
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)

	r.Get("/sensors", handleGetSensors)
	r.Get("/sensor/{id}", handleGetSensorID)
	r.Get("/station", handleGetStation)
	r.Get("/ssids", getSSIDs)
	r.Post("/connect-ssid", handlePostConnectSSID)
	r.Patch("/sensor/rename", handlePatchSensorRename)
	r.Patch("/sensor/full", handlePatchSensorFull)
	r.Patch("/hostname", handleHostRename)

	addr := restPort

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error(fmt.Sprintf("REST HTTP server error: %v", err))
		}
		logger.Debug("!!!!! Stopped serving new REST connections.")
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(fmt.Sprintf("HTTP shutdown error: %v", err))
	}
	logger.Debug("!!!!! HTTP REST Server graceful shutdown complete.")
}

/************** GET HANDLERS ******************/
func handleGetSensors(w http.ResponseWriter, r *http.Request) {
	rows, err := queryAllSensorsAsMaps(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Database error"})
		return
	}
	if len(rows) == 0 {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "No sensors found"})
		return
	}
	writeJSON(w, http.StatusOK, rows)
	logger.Debug(fmt.Sprintf("Returned %d sensors", len(rows)))
	logger.Debug(fmt.Sprintf("Returned sensors: %v", rows))
}

func handleGetSensorID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	item, err := querySensorByIDAsMap(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Database error"})
		return
	}
	if item == nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func handleGetStation(w http.ResponseWriter, r *http.Request) {
	// Station is a single global value stored in SENSORS.STATION
	// Use LIMIT 1 so we never scan the whole table.
	q := `SELECT STATION FROM SENSORS WHERE STATION IS NOT NULL AND TRIM(STATION) <> '' LIMIT 1`

	var station string
	err := db.QueryRowContext(r.Context(), q).Scan(&station)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			writeJSON(w, http.StatusNoContent, map[string]any{"error": "Station not set"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Database error"})
		return
	}

	station = strings.TrimSpace(station)
	if station == "" {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Station not set"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"STATION": station,
	})
}

func getSSIDs(w http.ResponseWriter, r *http.Request) {
	// Requires NetworkManager's nmcli.
	// It lists Wi-Fi networks visible right now.
	cmd := exec.CommandContext(r.Context(),
		"nmcli", "-t", "-f", "SSID", "dev", "wifi", "list",
	)

	out, err := cmd.Output()
	if err != nil {
		// Common errors: nmcli not installed, not running, no Wi-Fi device, etc.
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Failed to list networks",
		})
		return
	}

	lines := strings.Split(string(out), "\n")

	seen := make(map[string]struct{}, len(lines))
	ssids := make([]string, 0, len(lines))

	for _, s := range lines {
		ssid := strings.TrimSpace(s)
		if ssid == "" {
			continue
		}
		if _, ok := seen[ssid]; ok {
			continue
		}
		seen[ssid] = struct{}{}
		ssids = append(ssids, ssid)
	}

	// Optional: keep consistent behavior with other handlers
	if len(ssids) == 0 {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "No SSIDs found"})
		return
	}

	writeJSON(w, http.StatusOK, ssids)
}

/****************** POST HANDLERS ******************/
type connectSSIDReq struct {
	SSID     string `json:"SSID"`
	Password string `json:"password"`
}

func handlePostConnectSSID(w http.ResponseWriter, r *http.Request) {
	var req connectSSIDReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "JSON decode error"})
		return
	}

	ssid := strings.TrimSpace(req.SSID)
	pwd := req.Password // allow empty for open networks; enforce if you want

	if ssid == "" {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "SSID is required"})
		return
	}

	if err := connectToSSID(ssid, pwd); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Failed to connect",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Connect command sent",
		"SSID":    ssid,
	})

	// Persist WiFi credentials
	currentWiFi.SSID = ssid
	currentWiFi.PWD = "protected" // Do not log the actual password
	currentWiFi.saveSSIDPWD()

	// Restart avahi-daemon to update mDNS hostname
	if err := restartAvahiDaemon(); err != nil {
		logger.Error(fmt.Sprintf("Failed to restart avahi-daemon: %v", err))
	}
}

/****************** PATCH HANDLERS ******************/
type patchRenameReq struct {
	ID         int    `json:"ID"`
	SENSORNAME string `json:"SENSORNAME"`
}

func handlePatchSensorRename(w http.ResponseWriter, r *http.Request) {
	var req patchRenameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "JSON decode error"})
		return
	}

	if req.ID <= 0 {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	// Update only if exists
	ok, err := sensorExists(r.Context(), req.ID)
	if err != nil {
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Database error"})
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	qry := `UPDATE SENSORS SET SENSORNAME = ? WHERE id = ?`
	if _, err := execCtx(r.Context(), qry, req.SENSORNAME, req.ID); err != nil {
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Database error"})
		return
	}

	// Python returns: response + 200; message had literal {id}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":    fmt.Sprintf("Sensor %d updated", req.ID),
		"SENSORNAME": req.SENSORNAME,
	})
}

func handlePatchSensorFull(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "JSON decode error"})
		return
	}

	id, ok := parsePositiveInt(req["ID"])
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Invalid sensor id."})
		return
	}

	name, _ := req["SENSORNAME"].(string)
	location, _ := req["LOCATION"].(string)
	description, _ := req["DESCRIPTION"].(string)

	if location == "" {
		location = " "
	}
	if description == "" {
		description = " "
	}

	active := normalizeActive(req["ACTIVE"])

	ok, err := sensorExists(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Database error"})
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	qry := `UPDATE SENSORS SET (SENSORNAME, LOCATION, DESCRIPTION, ACTIVE) = (?,?,?,?) WHERE id = ?`
	if _, err := execCtx(r.Context(), qry, name, location, description, active, id); err != nil {
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Database error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":     "Sensor {id} updated",
		"SENSORNAME":  name,
		"LOCATION":    location,
		"DESCRIPTION": description,
		"ACTIVE":      active,
	})
}

type patchHostnameReq struct {
	HOSTNAME string `json:"HOSTNAME"`
}

func handleHostRename(w http.ResponseWriter, r *http.Request) {
	var req patchHostnameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "JSON decode error"})
		return
	}

	// Validate hostname
	logger.Debug(fmt.Sprintf("Validating hostname: %s", req.HOSTNAME))
	if err := validHostname(req.HOSTNAME); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": fmt.Sprintf("Invalid hostname: %v", err)})
		logger.Error(fmt.Sprintf("Invalid hostname: %v", err))
		return
	}

	// Set hostname
	if err := setHostname(req.HOSTNAME); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("Failed to set hostname: %v", err)})
		logger.Error(fmt.Sprintf("Failed to set hostname: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "Hostname updated",
		"HOSTNAME": req.HOSTNAME,
	})
}

// --- DB helpers (avoid “read whole table then find”) ---

func sensorExists(ctx context.Context, id int) (bool, error) {
	var exists int
	q := `SELECT 1 FROM SENSORS WHERE id = ? LIMIT 1`
	err := db.QueryRowContext(ctx, q, id).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func querySensorByIDAsMap(ctx context.Context, id int) (map[string]any, error) {
	q := fmt.Sprintf(`SELECT * FROM %s WHERE id = ?`, tableName)
	row := db.QueryRowContext(ctx, q, id)

	cols, err := columnNames(ctx)
	if err != nil {
		return nil, err
	}

	// Scan into []any using pointers
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	if err := rowScanInto(row, ptrs); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	out := make(map[string]any, len(cols))
	for i, c := range cols {
		v := vals[i]
		switch t := v.(type) {
		case int64:
			out[c] = int(t)
		case float64:
			out[c] = t
		case []byte:
			out[c] = string(t)
		default:
			out[c] = v
		}
	}
	return out, nil
}

func queryAllSensorsAsMaps(ctx context.Context) ([]map[string]any, error) {
	q := fmt.Sprintf(`SELECT * FROM %s`, tableName)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]any, len(cols))
		for i, c := range cols {
			v := vals[i]
			switch t := v.(type) {
			case int64:
				rowMap[c] = int(t)
			case float64:
				rowMap[c] = t
			case []byte:
				rowMap[c] = string(t)
			default:
				rowMap[c] = v
			}
		}
		results = append(results, rowMap)
	}
	return results, rows.Err()
}

func columnNames(ctx context.Context) ([]string, error) {
	// Quick way to get column names for SELECT *: query LIMIT 0
	q := fmt.Sprintf(`SELECT * FROM %s LIMIT 0`, tableName)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

func rowScanInto(row *sql.Row, ptrs []any) error {
	// sql.Row can be scanned only once; wrap for clean signature
	return row.Scan(ptrs...)
}

func execCtx(ctx context.Context, qry string, args ...any) (sql.Result, error) {
	stmt, err := db.PrepareContext(ctx, qry)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return stmt.ExecContext(ctx, args...)
}

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parsePositiveInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		i := int(t)
		return i, i > 0
	case int:
		return t, t > 0
	case string:
		s := strings.TrimSpace(t)
		i, err := strconv.Atoi(s)
		return i, err == nil && i > 0
	default:
		return 0, false
	}
}

func normalizeActive(v any) int {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		iv, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return iv
	default:
		return 0
	}
}
