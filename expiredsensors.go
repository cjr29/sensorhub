package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

func removeExpiredSensors(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Debug("!!!!! Shutting down remove expired sensors thread.")
			return // quit
		default:
			amt := 10 * 24 // seconds delay
			logger.Debug(fmt.Sprintf("Remove expired sensors every %d seconds", amt))
			runProcess()
			time.Sleep(time.Duration(amt) * time.Second)
		}
	}
}

type sensorRow map[string]any

func createJSONFromSQLite(dbPath, tableName, searchEnd string) (string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()

	// Equivalent to:
	// SELECT * FROM {table} WHERE SENSORNAME='UNKNOWN' AND TIME < '{search_end}';
	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE SENSORNAME = ? AND TIME < ?;",
		tableName,
	)

	rows, err := db.Query(query, "UNKNOWN", searchEnd)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var out []sensorRow
	for rows.Next() {
		// Create scan destinations dynamically.
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}

		m := make(sensorRow, len(cols))
		for i, c := range cols {
			m[c] = values[i]
		}
		out = append(out, m)
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func deleteSensor(dbPath, tableName string, id any) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Equivalent to:
	// DELETE FROM {table} WHERE ID = '{id}';
	query := fmt.Sprintf("DELETE FROM %s WHERE ID = ?;", tableName)
	_, err = db.Exec(query, id)
	return err
}

func runProcess() {

	tableName := "SENSORS"

	// Handle missing db case.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logger.Error("Expired Sensors Processing: No sensor database available. Terminate thread.")
		return
	}

	defer db.Close()

	// Process unknown sensor records older than 1 week.
	searchEnd := time.Now().Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	sensorsJSON, err := createJSONFromSQLite(dbPath, tableName, searchEnd)
	if err != nil {
		logger.Error(fmt.Sprintf("Database/JSON error: %v\n", err))
		return
	}

	var sensors []sensorRow
	if err := json.Unmarshal([]byte(sensorsJSON), &sensors); err != nil {
		logger.Error(fmt.Sprintf("JSON error: %v\n", err))
		return
	}

	for _, sensor := range sensors {
		targetSensor := sensor["ID"]
		targetModel := sensor["MODEL"]
		targetSensorID := sensor["SENSORID"]

		if err := deleteSensor(dbPath, tableName, targetSensor); err != nil {
			logger.Error(fmt.Sprintf("Database error deleting sensor ID %v: %v\n", targetSensor, err))
			continue
		}

		logger.Debug(fmt.Sprintf("Removed sensor ID %v - Model %v - id:%v\n", targetSensor, targetModel, targetSensorID))
	}

	logger.Info("Expired Sensors Processing: End cleanup of sensor records.")
}
