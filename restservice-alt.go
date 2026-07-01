// weatherREST.go
// REST API: GET /sensors, GET /sensor/{id}, PATCH /sensor/rename, PATCH /sensor/full
// Uses: modernc.org/sqlite + database/sql

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"
)

// --- main ---

func restServiceHandlerAlt() {
	// Match your Python default path
	dbPath := os.Getenv("WEATHER_DB")
	if dbPath == "" {
		dbPath = "/var/data/weather.db"
	}

	// modernc.org/sqlite DSN. We only need rw for an existing file.
	// modernc/sqlite supports "file:" URIs; ensure path is URL-escaped.
	esc := url.PathEscape(dbPath)
	dsn := fmt.Sprintf("file:%s?mode=rw", esc)

	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("Unable to open sqlite database: %v", err)
	}
	defer db.Close()

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Unable to ping sqlite database: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)

	r.Get("/sensors", handleGetSensors)
	r.Get("/sensor/{id}", handleGetSensorID)
	r.Patch("/sensor/rename", handlePatchSensorRename)
	r.Patch("/sensor/full", handlePatchSensorFull)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

// --- handlers ---

func handleGetSensorsA(w http.ResponseWriter, r *http.Request) {
	rows, err := querySensorsAsMaps(r.Context())
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

func handleGetSensorIDA(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	rows, err := querySensorsAsMaps(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Database error"})
		return
	}

	item := findElement(rows, "ID", id)
	if item != nil {
		writeJSON(w, http.StatusOK, item)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
}

type patchRenameReqA struct {
	ID         int    `json:"ID"`
	SENSORNAME string `json:"SENSORNAME"`
}

func handlePatchSensorRenameA(w http.ResponseWriter, r *http.Request) {
	var req patchRenameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "JSON decode error"})
		return
	}

	// Python does "find in full table first" then update; we match that behavior.
	rows, err := querySensorsAsMaps(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Database error"})
		return
	}
	found := findElement(rows, "ID", req.ID)
	if found == nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	qry := `UPDATE SENSORS SET SENSORNAME = ? WHERE id = ?`
	if _, err := execCtx(r.Context(), qry, req.SENSORNAME, req.ID); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Database error"})
		return
	}

	// Keep Python's message format (it used literal {id})
	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "Sensor {id} updated",
		"SENSORNAME": req.SENSORNAME,
	})
}

type patchFullReq struct {
	ID          int             `json:"ID"`
	SENSORNAME  string          `json:"SENSORNAME"`
	LOCATION    string          `json:"LOCATION"`
	DESCRIPTION string          `json:"DESCRIPTION"`
	ACTIVE      json.RawMessage `json:"ACTIVE"` // could be string/int/null
}

func handlePatchSensorFullA(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "JSON decode error"})
		return
	}

	// Validate ID like Python: integer > 0
	idVal, _ := req["ID"]
	id := 0
	switch v := idVal.(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	case string:
		iv, _ := strconv.Atoi(strings.TrimSpace(v))
		id = iv
	}
	if !(id > 0) {
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

	// Python's ACTIVE normalization: if null => 0; if string => string; else use value
	active := 0
	switch v := req["ACTIVE"].(type) {
	case nil:
		active = 0
	case float64:
		active = int(v)
	case int:
		active = v
	case string:
		// Python sets active = str(...) but then passes as param.
		// sqlite will store it in a numeric column if it can cast; we try to parse, else keep 0.
		if iv, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			active = iv
		} else {
			active = 0
		}
	default:
		active = 0
	}

	// Match Python: check existence by reading full table and searching by ID
	rows, err := querySensorsAsMaps(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Database error"})
		return
	}
	found := findElement(rows, "ID", id)
	if found == nil {
		w.WriteHeader(http.StatusNoContent)
		writeJSON(w, http.StatusNoContent, map[string]any{"error": "Requested sensor not found"})
		return
	}

	qry := `UPDATE SENSORS SET (SENSORNAME, LOCATION, DESCRIPTION, ACTIVE) = (?,?,?,?) WHERE id = ?`
	if _, err := execCtx(r.Context(), qry, name, location, description, active, id); err != nil {
		w.WriteHeader(http.StatusNoContent)
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

// --- sqlite helpers ---

func querySensorsAsMaps(ctx context.Context) ([]map[string]any, error) {
	// Python: SELECT * FROM SENSORS; returns JSON list of row dicts with cursor.description names.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", tableName))
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
			// Normalize numeric-ish types from sqlite driver to float64/int where possible.
			switch t := v.(type) {
			case int64:
				rowMap[c] = int(t)
			case float64:
				rowMap[c] = t
			case []byte:
				// sqlite may return text as []byte
				rowMap[c] = string(t)
			default:
				rowMap[c] = v
			}
		}
		results = append(results, rowMap)
	}
	return results, rows.Err()
}

func execCtxA(ctx context.Context, qry string, args ...any) (sql.Result, error) {
	stmt, err := db.PrepareContext(ctx, qry)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return stmt.ExecContext(ctx, args...)
}

func findElement(data []map[string]any, key string, value any) map[string]any {
	for _, item := range data {
		v, ok := item[key]
		if !ok {
			continue
		}
		if equalsLoose(v, value) {
			return item
		}
	}
	return nil
}

func equalsLoose(a any, b any) bool {
	switch av := a.(type) {
	case int:
		switch bv := b.(type) {
		case int:
			return av == bv
		case float64:
			return float64(av) == bv
		case string:
			iv, _ := strconv.Atoi(strings.TrimSpace(bv))
			return av == iv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case int:
			return av == float64(bv)
		}
	case string:
		switch bv := b.(type) {
		case string:
			return av == bv
		case int:
			iv, _ := strconv.Atoi(strings.TrimSpace(av))
			return iv == bv
		}
	}
	return errors.Is(fmt.Errorf("%v", a), fmt.Errorf("%v", b)) // fallback (rare)
}

// --- http/json helpers ---

func writeJSONA(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
