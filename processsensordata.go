package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "modernc.org/sqlite"
)

var (
	dbPath = "/var/sensorhub/weather.db"

	client mqtt.Client
)

type SensorPayload map[string]any

// func processSensorData(ctx context.Context) {
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			logger.Debug("!!!!! Shutting down sensor data processing thread.")
// 			return // quit
// 		default:
// 			// amt := 6 // seconds delay
// 			// time.Sleep(time.Duration(amt) * time.Second)
// 			// logger.Info(fmt.Sprintf("Processing sensor data every %d seconds", amt))
// 			logger.Debug("Starting Sensor Processing Service.")
// 			processSensorDataHandler()
// 		}
// 	}
// }

func processSensorData(ctx context.Context) {
	var err error
	db, err = initDB(dbPath)
	if err != nil {
		logger.Error(fmt.Sprintf("DB init failed: %v", err))
	}
	defer db.Close()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", "localhost", 1883))
	opts.SetClientID("ProcessSensorData")
	opts.SetAutoReconnect(true)

	opts.OnConnect = func(c mqtt.Client) {
		logger.Debug("Connected to MQTT broker: subscribing")

		subscribeTopics := []string{
			"sensorhub/home/weather/sensors/#",
		}
		for _, t := range subscribeTopics {
			if token := c.Subscribe(t, 0, onMessage); token.Wait() && token.Error() != nil {
				logger.Debug(fmt.Sprintf("Subscribe failed for %q: %v", t, token.Error()))
			}
		}
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		logger.Debug(fmt.Sprintf("MQTT connection lost: %v", err))
	}

	client = mqtt.NewClient(opts)
	logger.Debug("Attempting MQTT connect...")
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.Error(fmt.Sprintf("MQTT connect failed: %v", token.Error()))
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Sensor Data Processing shutting down...")
			_ = db.Close()
			client.Disconnect(250)
			return
		default:
			// do work
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func initDB(path string) (*sql.DB, error) {
	// modernc.org/sqlite registers driver name "sqlite"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := ensureTable(db); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS SENSORS (
		ID INTEGER PRIMARY KEY AUTOINCREMENT,
		TIME TEXT,
		SENSORID INTEGER,
		MODEL TEXT,
		CHANNEL TEXT,
		BATTERY INTEGER,
		TEMPERATURE NUMERIC,
		HUMIDITY NUMERIC,
		SENSORNAME TEXT,
		STATION TEXT,
		LOCATION TEXT,
		DESCRIPTION TEXT,
		ACTIVE INTEGER
	);`
	_, err := db.Exec(schema)
	return err
}

func onMessage(c mqtt.Client, m mqtt.Message) {
	topic := string(m.Topic())
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return
	}

	// sHub := parts[0]
	sStation := parts[1]
	sWeather := parts[2]
	sSensors := parts[3]

	if sWeather != "weather" || sSensors != "sensors" {
		return
	}

	var payload SensorPayload
	if err := json.Unmarshal(m.Payload(), &payload); err != nil {
		return
	}

	sensorID := toInt(payload["id"])

	// Query existing
	var (
		curID          int
		curTime        sql.NullString
		curSensorID    int
		curModel       sql.NullString
		curChannel     sql.NullString
		curBattery     sql.NullInt64
		curTemperature sql.NullFloat64
		curHumidity    sql.NullFloat64
		curSensorName  sql.NullString
		curLocation    sql.NullString
		curDescription sql.NullString
		curActive      sql.NullInt64
	)

	row := db.QueryRow(
		`SELECT ID, TIME, SENSORID, MODEL, CHANNEL, BATTERY, TEMPERATURE, HUMIDITY,
		        SENSORNAME, STATION, LOCATION, DESCRIPTION, ACTIVE
		   FROM SENSORS
		  WHERE SENSORID = ?`,
		sensorID,
	)

	// STATION is scanned into a dummy since we don't need the old one except Python used cur_station = results[0][??].
	var dummyStation sql.NullString
	err := row.Scan(
		&curID, &curTime, &curSensorID, &curModel, &curChannel, &curBattery,
		&curTemperature, &curHumidity, &curSensorName,
		&dummyStation,
		&curLocation, &curDescription, &curActive,
	)

	if err == sql.ErrNoRows {
		// Insert full record
		sModel := toString(payload["model"])
		sTime := toString(payload["time"])

		sChannel := " "
		if v, ok := payload["channel"]; ok {
			sChannel = toString(v)
		}

		sBattery := -1
		if v, ok := payload["battery_ok"]; ok {
			sBattery = toInt(v)
		}

		sTemperature := -999.9
		if v, ok := payload["temperature_F"]; ok {
			sTemperature = toFloat(v, -999.9)
		}

		sHumidity := -99.0
		if v, ok := payload["humidity"]; ok {
			sHumidity = toFloat(v, -99.0)
		}

		sensorName := "UNKNOWN"
		location := " "
		description := " "
		active := 0

		_, _ = db.Exec(
			`INSERT INTO SENSORS (TIME, SENSORID, MODEL, CHANNEL, BATTERY, TEMPERATURE, HUMIDITY, SENSORNAME, STATION, LOCATION, DESCRIPTION, ACTIVE)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			sTime, sensorID, sModel, sChannel, sBattery, sTemperature, sHumidity,
			sensorName, sStation, location, description, active,
		)
		return
	}
	if err != nil {
		return
	}

	// Update with overwrite rules
	curTempVal := nullFloatOrDefault(curTemperature, -999.9)
	curHumidVal := nullFloatOrDefault(curHumidity, -99.0)

	newTemperature := curTempVal
	if curTempVal != -999.9 {
		if v, ok := payload["temperature_F"]; ok {
			newTemperature = toFloat(v, curTempVal)
		}
	}

	newHumidity := curHumidVal
	if curHumidVal != -99.0 {
		if v, ok := payload["humidity"]; ok {
			newHumidity = toFloat(v, curHumidVal)
		}
	}

	newBattery := int(curBattery.Int64)
	if newBattery != -1 {
		if _, ok := payload["battery_ok"]; ok {
			newBattery = toInt(payload["battery_ok"])
		}
	}

	newTime := toString(payload["time"])

	_, _ = db.Exec(
		`UPDATE SENSORS SET TIME = ?, BATTERY = ?, TEMPERATURE = ?, HUMIDITY = ? WHERE ID = ?`,
		newTime, newBattery, newTemperature, newHumidity, curID,
	)

	_ = curModel
	_ = curChannel
	_ = curSensorName
	_ = curLocation
	_ = curDescription
	_ = curActive
	_ = curSensorID
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		if math.Trunc(x) == x {
			return fmt.Sprintf("%.0f", x)
		}
		return fmt.Sprintf("%v", x)
	case float32:
		f := float64(x)
		if math.Trunc(f) == f {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprintf("%v", f)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	default:
		var i int
		_, _ = fmt.Sscanf(fmt.Sprintf("%v", v), "%d", &i)
		return i
	}
}

func toFloat(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return def
		}
		return f
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return def
	}
}

func nullFloatOrDefault(n sql.NullFloat64, def float64) float64 {
	if !n.Valid {
		return def
	}
	return n.Float64
}
