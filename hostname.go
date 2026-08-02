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
// "sensorhubxxxxxx" where xxxxxx are the last six hex digits (last 3 bytes)
// of the MAC address of wlan0.
func EnsureSensorHubHostname() error {
	cur, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("read hostname: %w", err)
	}
	cur = strings.ToLower(strings.TrimSpace(cur))

	xxxxxx, err := lastSixHexDigitsOfMACInterface("wlan0")
	if err != nil {
		return err
	}

	want := "sensorhub" + xxxxxx
	if cur == want {
		return nil
	}

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

func lastSixHexDigitsOfMACInterface(ifName string) (string, error) {
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
	if len(mac) < 3 {
		return "", errors.New("MAC address too short")
	}

	// Last 3 bytes => 6 hex digits.
	last3 := mac[len(mac)-3:]
	return fmt.Sprintf("%02x%02x%02x", last3[0], last3[1], last3[2]), nil
}
