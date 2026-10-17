# sensorhub
Code for open source weather sensor hub on RaspberryPi Zero2W

## REST API transport modes

For compatibility, the REST API continues to use unauthenticated HTTP on port
5000 when no `REST_TLS_CERT`, `REST_TLS_KEY`, or `REST_BEARER_TOKEN` variables
are set. This legacy mode does not encrypt sensor data or Wi-Fi passwords and
should only be used on a trusted, isolated network.

To enable built-in HTTPS and bearer-token authentication, configure all three
variables for the sensorhub service:

- `REST_TLS_CERT`: path to the TLS certificate PEM file.
- `REST_TLS_KEY`: path to the matching private key PEM file.
- `REST_BEARER_TOKEN`: a random secret of at least 32 characters.

For example, generate a token with `openssl rand -hex 32`, then configure these
values using a root-readable systemd environment file. Keep the private key and
environment file readable only by the service administrator. Restart
sensorhub after configuring them. If only some of the variables are provided,
the REST service refuses to start rather than silently falling back to HTTP.

In secure mode, every endpoint requires the token in the `Authorization`
header. Do not put it in a URL or log it. For a private/self-signed certificate,
install or provide its CA certificate to the client:

```sh
curl --cacert sensorhub-ca.crt \
  -H "Authorization: Bearer $REST_BEARER_TOKEN" \
  https://sensorhub.local:5000/sensors
```

Clients must use HTTPS and add the bearer header to both read and write calls.
Removing the three variables restores the existing HTTP behavior for clients
that have not yet been updated.

We DO NOT USE bearer-token authentication in this implementation because
the sensorhub is local to the client. If we use a reverse tunnel like pinggy.io,
then the reverse tunnel provides the API security.

The systemd service script for sensorhub-hotspot redirects stdout to /var/log/sensorhub.log

Responds to curl commands:
 1) Rename sensorhub from 'sensorhub' to 'home' without reboot:
    curl -X PATCH -H "Content-Type: application/json" -d '{"HOSTNAME":"home"}' http://sensorhub.local:5000/hostname
 2) Rename a sensor:
    curl -X PATCH -H "Content-Type: application/json" -d '{"ID":26, "SENSORNAME":"Deck"}' http://sensorhub.local:5000/sensor/rename

 MAC Address of PI is embedded in all sensor records to uniquely identify the source of the record.
 Each sensor has a unique ID to support record updates.