# sensorhub
Code for open source weather sensor hub on RaspberryPi Zero2W

The systemd service script for sensorhub-hotspot redirects stdout to /var/log/sensorhub.log

Responds to curl commands:
 1) Rename sensorhub from 'sensorhub' to 'home' without reboot:
    curl -X PATCH -H "Content-Type: application/json" -d '{"HOSTNAME":"home"}' http://sensorhub.local:5000/hostname
 2) 

 MAC Address of PI is embedded in all sensor records to uniquely identify the source of the record.
 Each sensor has a unique ID to support record updates.