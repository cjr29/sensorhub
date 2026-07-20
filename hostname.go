package main

import (
	"errors"
	"fmt"
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
