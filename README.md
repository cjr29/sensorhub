    # SensorHub

A Raspberry Pi-based weather sensor hub for collecting, processing, and exposing environmental sensor data.

## Overview

SensorHub is designed to run on a Raspberry Pi Zero W and gather data from one or more environmental sensors, such as temperature, humidity, pressure, and other local weather metrics. It provides a simple way to collect readings, log them locally, and expose them over a lightweight API or dashboard in a vendor-independent manner. Sensors by AcuRite, LaCross, Oregon Scientific, and Ambient are supported. Others can easily be added.

This project is intended to be easy to set up, easy to extend, and suitable for personal weather monitoring, home automation experiments, and small local sensor networks.

## Features

- Raspberry Pi Zero W friendly
- Supports environmental sensor integrations
- Local data collection and logging
- Simple configuration model
- Extensible service structure
- Suitable for home and small-scale monitoring use cases

## Hardware Requirements

- Raspberry Pi Zero W or compatible Raspberry Pi
- Power supply suitable for the Pi
- Environmental sensors compatible with your chosen interface
- Optional: SD card, networking, enclosure, and sensor wiring

## Prerequisites

Before getting started, make sure you have:

- Raspberry Pi OS or another supported Linux environment
- Go installed
- Access to the GPIO or required sensor interfaces
- A working network connection for setup and monitoring

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/cjr29/sensorhub.git
   cd sensorhub

2. Detailed design and build instructions are contained in the PDF manual in the repository. Code is developed and maintained using Visual Studio Code running on an Apple Macbook running macOS 27.x. Code is cross-compiled on the Mac for deployment on the Raspberry Pi. Both a 64-bit version and a 32-version are built using the tasks.json file in the repository.

3. Once compiled, the code is committed to the repo from the Mac environment. Connect via SSH to the target Sensor Hub. Change to the `sensorhub` directory. Using git, pull the changes.

4. For 64-bit (PiZero2W), copy the executable `sensorhub` to `/usr/local/bin/`. For the 32-bit version, copy `sensorhubARMv6` to `/usr/local/bin/sensorhub`. All systemd service scripts expect executables to be named `sensorhub`.

5. See the Sensor Hub System Design Specification (SDS) in this repository for detailed build and installation instructions. This application uses MQTT message broker, RTL_433 open source software defined radio, avahi service for Bonjour advertising, and Raspberry Pi built-in hotspot service. Configuration and use of these services is described in the SDS.

# Design Elements
Code for open source weather sensor hub on RaspberryPi ZeroW.

## Two separate, communicating programs
There are two independent go applications that implement the Sensor Hub services. One is the sensorhubcontrol program. It's function is to control the hardware power and status LEDs as well as watch for rest button presses. sensorhubcontrol monitors a configuration file `/var/sensorhub/sensorhub.conf` and a LED status file `/var/sensorhub/led.state`. If the reset button is pressed for 5 seconds, sensorhubcontrol deletes the sensorhub.conf file causing sensorhub program to enter configuration mode and start the hotspot. sensorhubcontrol also watches the led.state file and sets the status LED based on the string value present in the file. The sensorhub program can cause the status light to change by updating the value in this file. Keeping the two programs separate facilitates real-time control of the Sensor Hub hardware while maintaining an independent sensor hub function.

## Use of Bonjour services
sensorhub advertises its availablity using a generally available service called Bonjour. Bonjour is Apple's implementation of zero-configuration networking. Apple has made the source code of the Bonjour multicast DNS responder, the core component of service discovery, available as a Darwin open source project. The project provides source code to build the responder daemon for a wide range of platforms, including Mac OS 9, macOS, Linux, *BSD, Solaris, VxWorks, and Windows.

Bonjour locates devices such as printers, other computers, and the services that those devices offer on a local network using multicast Domain Name System (mDNS) service records. This means that a Sensor Hub can be readily discovered on a LAN without the client polling every device on the network. Using Bonjour also makes implementing a client on Apple iOS practical because iOS typically does not permit a dvice (iPhone) to poll all netowkr devices. The iOS client just needs to look for a Bonjour device advertising as `_sensorhub._tcp`. The avahi daemon running on Raspberry Pi advertises the Sensor Hub.

## REST API
All data collected by the Sensor Hub's radio is processed in real-time. Validated sensor data is inserted (upserted) into a sqlite3 database stored at `\var\sensorhub\weather.db`. Each sensor managed by the Sensor Hub has its own record in the database. As sensor values are updated, sensorhub program replaces the latest values for the record in the database. Only one record per sensor is maintained.

Clients request lists of Sensor Hubs and Sensors using a simple REST API. Clients can also manage the Sensor records by renaming them, adding location details, and including descriptions.

The REST API can be invoked from the command line using `curl`, from a Browser using a url, and from an Apple iOS client (which internally uses the REST API). API endpoints are available for lists of Sensors. Endpoints are available for POSTing updates to Sensor records. The sqlite3 database stores Sensor records using the unique hostname assigned to each Sensor Hub, e.g. `sensorhub88a29e330a21`. Multiple Sensor Hubs can be present on a LAN. Each Hub has its own copy of the Sensor records identified by its hostname.

## Reverse proxy service for enabling remote access from anywhere
The basic sensorhub application connects the Hub to a local network via WiFi. To access the sensors it collects, your must be on a computer or other device connected to the same WiFi. To gain access to the Hub remotely outside the local WiFi, the Hub must be visible to DNS across the Internet. 

There are multiple ways to make this happen. One is to get a dedicated IP assigned to the Hub. That requires having a domain name and then assigning an IP address to the Hub. Another way is to use a VPN service and access the Hub through the VPN. A third way is to use a low-cost reverse proxy service that creates a virtual pipe between the Hub and the service provider. You connect your computer or device to the service provider through a dedicated https session. The service routes communications to and from the Hub through it's virtual tunnel. The reverse proxy is usually the cheapest and easiest way to gain remote access without hosting your own domain and server or paying for an expensive VPN.

sensorhub has been tested with a reverse proxy service from pinggy.io. Cost is a reasonable $30 per year at the time of this writing (2026). The necessary scripts are included in this repository along with step-by-step instructions for configure the reverse proxy in the system design specification PDF.

## REST API transport modes security - Default is open

For compatibility, the REST API continues to use unauthenticated HTTP on port
5000 when no `REST_TLS_CERT`, `REST_TLS_KEY`, or `REST_BEARER_TOKEN` variables
are set. This legacy mode does not encrypt sensor data or Wi-Fi passwords and
should only be used on a trusted, isolated network.

To enable the built-in HTTPS and bearer-token authentication, configure all three
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

## Browser Wi-Fi setup

When `/var/sensorhub/sensorhub.conf` is missing, SensorHub starts its default
`SensorHub` hotspot. Join that network from a phone or computer and open
`http://10.42.0.1:5000/` in a browser to scan for networks and configure Wi-Fi.
The browser tool uses the same `/ssids` and `/connect-ssid` REST endpoints as
the iOS client. After a successful setup, reconnect your device to the
configured Wi-Fi network.

Responds to curl commands:
 1) Rename sensorhub from 'sensorhub' to 'home' without reboot:
    curl -X PATCH -H "Content-Type: application/json" -d '{"HOSTNAME":"home"}' http://sensorhub.local:5000/hostname
 2) Rename a sensor:
    curl -X PATCH -H "Content-Type: application/json" -d '{"ID":26, "SENSORNAME":"Deck"}' http://sensorhub.local:5000/sensor/rename

 MAC Address of PI is embedded in all sensor records to uniquely identify the source of the record.
 Each sensor has a unique ID to support record updates.

 ## iPhone Wi-Fi setup

 A sibling open source project on github https://github.com/cjr29/Sensor-Hub-Xcode.git provides an Apple iOS application that enables
 a rich environment for displaying the sensor data. It also supports management of multiple sensor hubs. Remote hubs are easily added.

 # Future Enhancements
 This application is expandable. Future updates include:
 - Add rain and wind data
 - Include local weather service forecast data by GPS
 - iOS Watch app

 See the Issues in github for both `cjr29/sensorhub.git` and `cjr29/Sensor-Hub-Xcode.git` for the latest plans.
 
 # Contributing

Contributions are welcome.
Please open an issue or submit a pull request with a clear description of:
- the problem being solved
- the changes made
- any configuration or compatibility considerations