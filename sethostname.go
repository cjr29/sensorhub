package main

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var hostLabelRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

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

	// Force the kernel to update its idea of the hostname immediately.
	cmd = exec.Command("sudo", "systemctl", "reboot")
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostname command failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func SetPrettyHostname(pretty string) error {
	pretty = strings.TrimSpace(pretty)
	if pretty == "" {
		return fmt.Errorf("pretty hostname is empty")
	}

	// --pretty sets PRETTY_HOSTNAME in /etc/machine-info
	cmd := exec.Command("hostnamectl", "set-hostname", "--pretty", pretty)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostnamectl failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// func main() {
// 	if err := SetPrettyHostname("My Server"); err != nil {
// 		panic(err)
// 	}
// }
