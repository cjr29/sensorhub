package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

func removeExpiredSensors(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logger.Debug("!!!!! Shutting down remove expired sensors thread.")
			return // quit
		default:
			amt := 7200 // seconds delay
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

// deleteExpiredSensors deletes records that meet any of these criteria:
//
//  1. TIME is older than 24 hours
//  2. SENSORNAME is "UNKNOWN" and TIME is older than 1 hour
//  3. ACTIVE is 0 and TIME is older than 1 hour
//
// TIME must be stored in a SQLite-compatible ISO-8601 format, for example:
//
//	2026-09-11 12:30:00
//	2026-09-11T12:30:00Z
func deleteExpiredSensors(ctx context.Context, db *sql.DB) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}

	defer func() {
		// Rollback is harmless if the transaction has already been committed.
		_ = tx.Rollback()
	}()

	const query = `
		DELETE FROM SENSORS
		WHERE
			datetime(TIME) < datetime('now', '-24 hours')
			OR (
				SENSORNAME = 'UNKNOWN'
				AND datetime(TIME) < datetime('now', '-1 hour')
			)
			OR (
				ACTIVE = 0
				AND datetime(TIME) < datetime('now', '-1 hour')
			);
	`

	result, err := tx.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func runProcess() {

	// tableName := "SENSORS"

	// Handle missing db case.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logger.Error("Expired Sensors Processing: No sensor database available. Terminate thread.")
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deleted, err := deleteExpiredSensors(ctx, db)
	if err != nil {
		logger.Error("Expired Sensors Processing: Could not delete expired sensor records.")
		return
	}

	logger.Info(fmt.Sprintf("Expired Sensors Processing: Deleted %d expired sensor records.", deleted))
}
