package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"
)

const tableName = "SENSORS"
const restPort = ":5000"

// const pinggyTunnelFile = "/usr/local/bin/test_tunnel.sh"
const pinggyTunnelFile = "/usr/local/bin/pinggy_tunnel.sh"
const maxRESTRequestBody = 64 << 10

var db *sql.DB
var pinggyTunnelMu sync.Mutex

type restSecurityConfig struct {
	certFile string
	keyFile  string
	token    string
}

func loadRESTSecurityConfig() (restSecurityConfig, error) {
	config := restSecurityConfig{
		certFile: os.Getenv("REST_TLS_CERT"),
		keyFile:  os.Getenv("REST_TLS_KEY"),
		token:    os.Getenv("REST_BEARER_TOKEN"),
	}
	if config.certFile == "" && config.keyFile == "" && config.token == "" {
		return config, nil
	}
	if config.certFile == "" || config.keyFile == "" || config.token == "" {
		return restSecurityConfig{}, errors.New("REST_TLS_CERT, REST_TLS_KEY, and REST_BEARER_TOKEN must all be set together")
	}
	if len(config.token) < 32 {
		return restSecurityConfig{}, errors.New("REST_BEARER_TOKEN must contain at least 32 characters")
	}
	return config, nil
}

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

type pinggyTunnelRequest struct {
	Value string `json:"value"`
}

func restService(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger, lightCmd chan int) {
	defer wg.Done()

	security, err := loadRESTSecurityConfig()
	if err != nil {
		logger.Error("Invalid REST security configuration", "err", err)
		return
	}

	esc := url.PathEscape(dbPath)
	dsn := fmt.Sprintf("file:%s?mode=rw", esc)
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		logger.Error("Unable to open sqlite database", "err", err)
		return
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		logger.Error("Unable to ping sqlite database", "err", err)
		return
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(securityHeaders(security.token != ""))
	if security.token != "" {
		r.Use(bearerTokenAuth(security.token))
	}

	r.Get("/", handleWiFiSetupPage)
	r.Get("/sensors", handleGetSensors)
	r.Get("/sensor/{id}", handleGetSensorID)
	r.Get("/station", handleGetStation)
	r.Get("/ssids", getSSIDs)
	r.Post("/connect-ssid", handlePostConnectSSID)
	r.Post("/pinggy-tunnel", handlePostPinggyTunnel)
	r.Patch("/sensor/rename", handlePatchSensorRename)
	r.Patch("/sensor/full", handlePatchSensorFull)
	r.Patch("/hostname", handleHostRename)

	addr := restPort
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	serveErr := make(chan error, 1)
	go func() {
		if security.token != "" {
			serveErr <- server.ListenAndServeTLS(security.certFile, security.keyFile)
		} else {
			serveErr <- server.ListenAndServe()
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("REST server stopped unexpectedly", "err", err)
		}
	}

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown error", "err", err)
	}
	logger.Debug("!!!!! HTTP REST Server graceful shutdown complete.")
}

//go:embed setup.html
var wifiSetupPage []byte

func handleWiFiSetupPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(wifiSetupPage); err != nil {
		logger.Error("Unable to serve Wi-Fi setup page", "err", err)
	}
}

func securityHeaders(enableHSTS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			if enableHSTS {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerTokenAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			values := r.Header.Values("Authorization")
			if len(values) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
				return
			}

			scheme, provided, found := strings.Cut(values[0], " ")
			if !found || !strings.EqualFold(scheme, "Bearer") ||
				subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRESTRequestBody)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body contains multiple JSON values")
		}
		return err
	}
	return nil
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
	// logger.Debug(fmt.Sprintf("Returned sensors: %v", rows))
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

	if len(ssids) == 0 {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "No SSIDs found"})
		return
	}

	writeJSON(w, http.StatusOK, ssids)
}

/****************** POST HANDLERS ******************/
const (
	maxConnectAttempts = 3
	connectRetryDelay  = 3 * time.Second
)

type connectSSIDReq struct {
	SSID     string `json:"SSID"`
	Password string `json:"password"`
}

func handlePostConnectSSID(w http.ResponseWriter, r *http.Request) {
	var req connectSSIDReq
	logger.Debug(fmt.Sprintf("Received Wi-Fi connection request: SSID=%q", strings.TrimSpace(req.SSID)))
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "JSON decode error",
		})
		return
	}

	ssid := strings.TrimSpace(req.SSID)
	pwd := req.Password // Empty passwords are allowed for open networks.

	if ssid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "SSID is required",
		})
		return
	}

	var connectErr error

	for attempt := 1; attempt <= maxConnectAttempts; attempt++ {
		connectErr = connectToSSID(ssid, pwd)
		if connectErr == nil {
			break
		}

		logger.Error("Failed to connect to Wi-Fi", "attempt", attempt, "max_attempts", maxConnectAttempts)

		if attempt < maxConnectAttempts {
			select {
			case <-time.After(connectRetryDelay):
			case <-r.Context().Done():
				writeJSON(w, http.StatusRequestTimeout, map[string]any{
					"error": "Connection attempt canceled",
				})
				return
			}
		}
	}

	if connectErr != nil {
		logger.Info("Failed to connect to Wi-Fi after three attempts", "err", connectErr)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Failed to connect after three attempts",
		})
		return
	}

	// Persist the SSID only after a successful connection. NetworkManager stores the password.
	currentWiFi.SSID = ssid
	if err := currentWiFi.saveSSID(); err != nil {
		logger.Error("Unable to persist Wi-Fi SSID", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Connected, but failed to persist Wi-Fi SSID",
		})
		return
	}

	// Restart avahi-daemon to update the mDNS hostname.
	if err := restartAvahiDaemon(); err != nil {
		logger.Error(fmt.Sprintf(
			"Failed to restart avahi-daemon: %v",
			err,
		))
	}

	lightCmd <- GREENON

	logger.Info(fmt.Sprintf("Successfully connected to Wi-Fi SSID %q", ssid))
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Connected successfully",
		"SSID":    ssid,
	})
}

func handlePostPinggyTunnel(w http.ResponseWriter, r *http.Request) {
	var req pinggyTunnelRequest

	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "Request body must be JSON: {\"value\":\"...\"}", http.StatusBadRequest)
		return
	}

	pinggyTunnelMu.Lock()
	defer pinggyTunnelMu.Unlock()

	original, err := os.ReadFile(pinggyTunnelFile)
	if err != nil {
		logger.Error("Unable to read tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	if !strings.Contains(string(original), "<placeholder>") {
		logger.Error("Placeholder was not found in tunnel script")
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	info, err := os.Stat(pinggyTunnelFile)
	if err != nil {
		logger.Error("Unable to stat tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	updated := strings.ReplaceAll(
		string(original),
		"<placeholder>",
		req.Value,
	)

	// Create the temporary file in the same directory so Rename is atomic.
	tempFile, err := os.CreateTemp("/usr/local/bin", ".pinggy_tunnel.sh-*")
	if err != nil {
		logger.Error("Unable to create temporary tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	tempName := tempFile.Name()
	defer os.Remove(tempName)

	// Preserve the original permission bits.
	if err := tempFile.Chmod(info.Mode().Perm()); err != nil {
		_ = tempFile.Close()
		logger.Error("Unable to set tunnel script permissions", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	if _, err := tempFile.WriteString(updated); err != nil {
		_ = tempFile.Close()
		logger.Error("Unable to write temporary tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		logger.Error("Unable to sync temporary tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	if err := tempFile.Close(); err != nil {
		logger.Error("Unable to close temporary tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	if err := os.Rename(tempName, pinggyTunnelFile); err != nil {
		logger.Error("Unable to replace tunnel script", "err", err)
		http.Error(w, "Unable to update tunnel configuration", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "updated",
	})
}

/****************** PATCH HANDLERS ******************/
type patchRenameReq struct {
	ID         int    `json:"ID"`
	SENSORNAME string `json:"SENSORNAME"`
}

func handlePatchSensorRename(w http.ResponseWriter, r *http.Request) {
	var req patchRenameReq
	if err := decodeJSONBody(w, r, &req); err != nil {
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
	if err := decodeJSONBody(w, r, &req); err != nil {
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
	if err := decodeJSONBody(w, r, &req); err != nil {
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
