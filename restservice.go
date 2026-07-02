package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// func restService(ctx context.Context) {
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			logger.Info("!!!!! Shutting down REST service thread.")
// 			return // quit
// 		default:
// 			// amt := 3600 // seconds delay
// 			// time.Sleep(time.Duration(amt) * time.Second)
// 			// logger.Info(fmt.Sprintf("REST service still running (checked every %d seconds) ...", amt))
// 			logger.Debug("Starting REST Service.")
// 			restServiceHandler()
// 		}
// 	}
// }

// weatherREST.go
// REST API: GET /sensors, GET /sensor/{id}, PATCH /sensor/rename, PATCH /sensor/full
// Uses: modernc.org/sqlite + database/sql

func restService(ctx context.Context) {
	esc := url.PathEscape(dbPath)
	dsn := fmt.Sprintf("file:%s?mode=rw", esc)
	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to open sqlite database: %v", err))
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		logger.Error(fmt.Sprintf("Unable to ping sqlite database: %v", err))
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)

	r.Get("/sensors", handleGetSensors)
	r.Get("/sensor/{id}", handleGetSensorID)
	r.Patch("/sensor/rename", handlePatchSensorRename)
	r.Patch("/sensor/full", handlePatchSensorFull)

	addr := restPort
	// if v := os.Getenv("PORT"); v != "" {
	// 	addr = ":" + v
	// }
	logger.Debug(fmt.Sprintf("Listening on %s", addr))
	err = http.ListenAndServe(addr, r)
	if err != nil {
		logger.Error(fmt.Sprintf("Error listening on port %s: %s", addr, err))
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("REST Service shutting down...")
			_ = db.Close()
			client.Disconnect(250)
			return
		default:
			// do work
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// --- handlers ---

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

	// Update only if exists (Python: reads then updates)
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
		"message":    "Sensor {id} updated",
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
	// SELECT * to preserve Python behavior of returning all columns as a map
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
