package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var hostLabelRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// Hostname returns the current system hostname (Linux).
func getHostname() (string, error) {
	// Most reliable/simple:
	h, err := os.Hostname()
	if err == nil {
		return strings.TrimSpace(h), nil
	}

	// If needed, you can add a fallback reading /etc/hostname (optional).
	// b, err2 := os.ReadFile("/etc/hostname")
	// if err2 != nil {
	//     return "", err // prefer the original os.Hostname error
	// }
	// return strings.TrimSpace(string(b)), nil

	return "", err
}

func validHostname(h string) error {
	h = strings.TrimSpace(h)
	if h == "" {
		return errors.New("hostname is empty")
	}
	// Simple RFC-ish sanity: allow single label (hostnamectl usually accepts full hostnames too)
	// If you want multi-label FQDN validation, tell me and I’ll adapt.
	if strings.Contains(h, ".") {
		// validate each label
		for _, part := range strings.Split(h, ".") {
			if part == "" || !hostLabelRE.MatchString(part) {
				return fmt.Errorf("invalid hostname label: %q", part)
			}
		}
		return nil
	}
	if !hostLabelRE.MatchString(h) {
		return fmt.Errorf("invalid hostname: %q", h)
	}
	return nil
}

func setHostname(hostname string) error {
	hostname = strings.TrimSpace(hostname)
	if err := validHostname(hostname); err != nil {
		return err
	}

	// Needs root (run the program as root or with sudo).
	// --static and --transient cover the common cases.
	cmd := exec.Command("sudo",
		"hostnamectl",
		"set-hostname",
		"--static",
		"--transient",
		hostname,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostnamectl failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	// Restart avahi-daemon to update the hostname immediately on the local network.
	cmd = exec.Command("sudo", "systemctl", "restart", "avahi-daemon")
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostname command failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// EnsureSensorHubHostname checks hostname format and renames the Pi to
// "sensorhubxxxxxx" where xxxxxxxxxxxxxx is the MAC address of wlan0. It returns an error if the hostname is invalid or if it fails to set the hostname. It requires root privileges to change the hostname and restart avahi-daemon.
// of the MAC address of wlan0.

func EnsureSensorHubHostname() error {
	cur, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("read hostname: %w", err)
	}
	cur = strings.ToLower(strings.TrimSpace(cur))
	macSuffix, err := lastTwelveHexDigitsOfMACInterface("wlan0")

	want := "sensorhub" + macSuffix // e.g. sensorhub00:11:22:33:44:55
	// hostnamectl typically requires root.
	cmd := exec.Command("sudo",
		"hostnamectl",
		"set-hostname",
		"--static",
		"--transient",
		want)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hostnamectl failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	// Restart avahi-daemon to update the hostname immediately on the local network.
	cmd = exec.Command("sudo", "systemctl", "restart", "avahi-daemon")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostname command failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func lastTwelveHexDigitsOfMACInterface(ifName string) (string, error) {
	// no colons, just the last 12 hex digits of the MAC address of the specified interface.
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}

	var mac net.HardwareAddr
	for _, ifc := range ifaces {
		if ifc.Name == ifName {
			mac = ifc.HardwareAddr
			break
		}
	}
	if len(mac) == 0 {
		return "", fmt.Errorf("no MAC address found for interface %q", ifName)
	}
	if len(mac) < 6 {
		return "", errors.New("MAC address too short")
	}

	// Entire MAC, rendered as 12 lowercase hex chars (colons removed).
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]), nil
}

func macWithColonsOfMACInterface(ifName string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}

	var mac net.HardwareAddr
	for _, ifc := range ifaces {
		if ifc.Name == ifName {
			mac = ifc.HardwareAddr
			break
		}
	}
	if len(mac) == 0 {
		return "", fmt.Errorf("no MAC address found for interface %q", ifName)
	}
	if len(mac) < 6 {
		return "", errors.New("MAC address too short")
	}

	// Use only first 6 bytes if the HardwareAddr is longer (rare, but safe).
	mac = mac[:6]

	// net.HardwareAddr.String() returns colon-separated hex, e.g. "00:11:22:33:44:55".
	return mac.String(), nil
}
