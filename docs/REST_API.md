# SensorHub REST API

This document describes the HTTP API implemented by SensorHub. It is intended
for clients written in any language that can make HTTP requests and process
JSON.

## Connecting to a hub

The service listens on TCP port `5000`. Use the hub's hostname or IP address
as the base URL, for example:

```text
http://sensorhub.local:5000
```

During Wi-Fi setup, connect to the SensorHub hotspot and use:

```text
http://10.42.0.1:5000
```

The hostname may be discoverable on the local network through Bonjour/mDNS.
Every hub has its own sensor database. Requests below are relative to the base
URL and do not require a trailing slash.

## Transport and authentication

By default, the service uses unauthenticated HTTP. HTTP does not encrypt
requests or responses; do not send Wi-Fi passwords over an untrusted network.

HTTPS and bearer-token authentication are enabled together by setting all of
these environment variables for the SensorHub service:

- `REST_TLS_CERT`: path to the TLS certificate PEM file.
- `REST_TLS_KEY`: path to its private key PEM file.
- `REST_BEARER_TOKEN`: a token at least 32 characters long.

If any but not all of the variables are set, the REST service does not start.
Secure mode uses HTTPS (TLS 1.2 or newer) and requires this header on every
request, including reads. Construct its value from these components in order:
the scheme name `Bearer`, one space, and the configured token.

For a private certificate authority, configure the client to trust that CA.
Never put the bearer token in a URL. The built-in browser Wi-Fi setup page does
not provide a way to enter a bearer token, so use the page with the default
unauthenticated setup mode.

## General behavior

- Send JSON request bodies with `Content-Type: application/json`.
- Successful JSON responses use `Content-Type: application/json`.
- JSON object field names are uppercase where shown below; send those exact
  names for compatibility.
- JSON request bodies are limited to 64 KiB and must contain exactly one JSON
  value.
- There is no pagination or filtering on the sensor-list endpoint.
- An empty result or several validation/not-found errors use HTTP `204 No
  Content`. Per HTTP semantics, a 204 response has no response body; clients
  should use the status code rather than expect the handler's attempted JSON
  error body.
- Other errors generally use a JSON object with an `error` field. The tunnel
  update endpoint uses plain-text errors.

## Endpoints

### `GET /`

Returns the built-in HTML Wi-Fi setup page (`text/html; charset=utf-8`).

### `GET /sensors`

Returns every sensor record as a JSON array:

```json
[
  {
    "ID": 26,
    "TIME": "source timestamp",
    "SENSORID": 12345,
    "MODEL": "example",
    "CHANNEL": "A",
    "BATTERY": 1,
    "TEMPERATURE": 21.5,
    "HUMIDITY": 48.2,
    "SENSORNAME": "Deck",
    "STATION": "Home",
    "LOCATION": "Deck",
    "DESCRIPTION": "Outside sensor",
    "ACTIVE": 1
  }
]
```

The example values are illustrative. Each object contains the database columns:

| Field | Type | Meaning |
|---|---|---|
| `ID` | integer | Unique database record ID; use this for record updates. |
| `TIME` | string | Source timestamp of the latest stored reading. The API does not normalize its format. |
| `SENSORID` | integer | Sensor's radio ID. |
| `MODEL` | string | Sensor model/type. |
| `CHANNEL` | string | Sensor channel. |
| `BATTERY` | integer | Battery indicator reported for the sensor. |
| `TEMPERATURE` | number | Latest temperature reading in Fahrenheit. |
| `HUMIDITY` | number | Latest relative-humidity reading. |
| `SENSORNAME` | string | User-assigned display name. |
| `STATION` | string | Station/hub name associated with the record. |
| `LOCATION` | string | User-assigned location. |
| `DESCRIPTION` | string | User-assigned description. |
| `ACTIVE` | integer | Active flag; `0` is inactive and nonzero values are active. |

Database `NULL` values are returned as JSON `null`. Numeric values may be
integers or decimal numbers depending on the column and reading.

Responses:

- `200 OK`: array of records.
- `204 No Content`: no sensor records.
- `500 Internal Server Error`: `{"error":"Database error"}`.

### `GET /sensor/{id}`

Returns one sensor record using its positive database `ID`, with the same
fields as an item in `GET /sensors`.

Examples:

```text
GET /sensor/26
GET /sensor/999
```

Responses:

- `200 OK`: the matching record as a JSON object.
- `204 No Content`: invalid/non-positive ID or no matching record.
- `500 Internal Server Error`: `{"error":"Database error"}`.

### `GET /station`

Returns the first non-empty station name found in the sensor table:

```json
{"STATION":"Home"}
```

Responses:

- `200 OK`: station object.
- `204 No Content`: no station is set.
- `500 Internal Server Error`: `{"error":"Database error"}`.

### `GET /ssids`

Scans for currently visible Wi-Fi networks and returns a de-duplicated JSON
array of SSID strings:

```json
["Home WiFi", "Guest WiFi"]
```

Responses:

- `200 OK`: array of SSIDs.
- `204 No Content`: no SSIDs found.
- `500 Internal Server Error`: `{"error":"Failed to list networks"}`; for
  example, the scan utility may be unavailable or the Wi-Fi device may not be
  ready.

### `POST /connect-ssid`

Connects the hub to the requested Wi-Fi network. The SSID is trimmed of
surrounding whitespace. An empty password is allowed for an open network.
Connection is attempted up to three times, with a three-second delay between
failed attempts. On success, the SSID is persisted in the hub configuration.

Request:

```json
{
  "SSID": "Home WiFi",
  "password": "example-password"
}
```

Responses:

- `200 OK`: `{"message":"Connected successfully","SSID":"Home WiFi"}`.
- `400 Bad Request`: invalid JSON (`{"error":"JSON decode error"}`) or an
  empty/missing SSID (`{"error":"SSID is required"}`).
- `408 Request Timeout`: request canceled while waiting between attempts
  (`{"error":"Connection attempt canceled"}`).
- `500 Internal Server Error`: failed connection after three attempts, or
  connection succeeded but saving the SSID failed.

This operation changes the hub's network connection. A client connected via
the setup hotspot may lose its connection to the API after a successful
request.

### `POST /pinggy-tunnel`

Replaces every `<placeholder>` occurrence in the hub's
`/usr/local/bin/pinggy_tunnel.sh` script with the supplied string. This is
intended to configure the Pinggy tunnel script; the hub must have that script
and placeholder installed. The value is inserted as-is, so only send a
trusted, correctly formatted configuration value. `value` should be a string;
if omitted or `null`, it is treated as an empty string.

Request:

```json
{"value":"your-tunnel-configuration"}
```

Responses:

- `200 OK`: `{"status":"updated"}`.
- `400 Bad Request`: invalid JSON; the response is plain text.
- `500 Internal Server Error`: the script cannot be read or updated, or no
  placeholder exists; the response is plain text.

### `PATCH /sensor/rename`

Updates only the display name for an existing sensor. The `ID` must be a
positive database ID. `SENSORNAME` may be an empty string.

Request:

```json
{"ID":26,"SENSORNAME":"Patio"}
```

Responses:

- `200 OK`: `{"message":"Sensor 26 updated","SENSORNAME":"Patio"}`.
- `204 No Content`: invalid JSON, invalid or unknown ID, or a database error.

### `PATCH /sensor/full`

Updates the display name, location, description, and active flag for an
existing sensor. All four fields are written on every request:

```json
{
  "ID": 26,
  "SENSORNAME": "Patio",
  "LOCATION": "East wall",
  "DESCRIPTION": "Sheltered from rain",
  "ACTIVE": 1
}
```

`ID` must resolve to a positive integer; a positive numeric string is also
accepted. Numeric JSON values are converted to integers.
`ACTIVE` is normalized to an integer; omitted or unparseable values become `0`.
If `LOCATION` or `DESCRIPTION` is missing or empty, it is stored as a single
space. Missing or non-string name/location/description values are treated as
empty strings (with the location/description fallback described above).

Responses:

- `200 OK`: updated values in a JSON object, for example:
  `{"message":"Sensor {id} updated","SENSORNAME":"Patio","LOCATION":"East wall","DESCRIPTION":"Sheltered from rain","ACTIVE":1}`.
  The literal `{id}` in the message is the current response text.
- `204 No Content`: invalid JSON, invalid or unknown ID, or a database error.

### `PATCH /hostname`

Changes the system hostname and restarts the Avahi daemon so the local
network's mDNS name is updated. Hostnames may contain dot-separated labels;
each label must be 1 to 63 characters, start and end with an ASCII letter or
digit, and may contain hyphens internally.

Request:

```json
{"HOSTNAME":"sensorhub-kitchen"}
```

Responses:

- `200 OK`: `{"message":"Hostname updated","HOSTNAME":"sensorhub-kitchen"}`.
- `204 No Content`: invalid JSON or invalid hostname.
- `500 Internal Server Error`: setting the hostname or restarting Avahi
  failed.

## Example requests

The following examples use `curl`; any HTTP client can send the same method,
path, headers, and JSON body:

```sh
BASE_URL="http://sensorhub.local:5000"

curl "$BASE_URL/sensors"
curl "$BASE_URL/sensor/26"
curl "$BASE_URL/station"

curl -X PATCH "$BASE_URL/sensor/rename" \
  -H "Content-Type: application/json" \
  -d '{"ID":26,"SENSORNAME":"Patio"}'

curl -X PATCH "$BASE_URL/sensor/full" \
  -H "Content-Type: application/json" \
  -d '{"ID":26,"SENSORNAME":"Patio","LOCATION":"East wall","DESCRIPTION":"Outside","ACTIVE":1}'
```

In secure mode, use an `https://` base URL and include the `Authorization`
header described above on every request. Configure `curl` with the appropriate
CA certificate when the hub uses a private certificate.

