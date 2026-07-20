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

// // HostnameUname returns the current system hostname using uname(2).
// func hostnameUname() (string, error) {
// 	var uts unix.Utsname
// 	if err := unix.Uname(&uts); err != nil {
// 		return "", err
// 	}
// 	// nodename is the hostname field for uname.
// 	hn := unix.ByteSliceToString(uts.Nodename[:])
// 	return strings.TrimRight(hn, "\x00 "), nil
// }

// // SetHostname sets the system hostname (Linux).
// // Note: typically requires appropriate privileges (e.g., root).
// func setHostname(name string) error {
// 	name = strings.TrimSpace(name)
// 	return unix.Sethostname([]byte(name))
// }

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
		// "--transient",
		hostname,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostnamectl failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	// // Force the kernel to update its idea of the hostname immediately.
	// cmd = exec.Command("sudo", "systemctl", "reboot")
	// out, err = cmd.CombinedOutput()
	// if err != nil {
	// 	return fmt.Errorf("hostname command failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	// }

	return nil
}

// func SetPrettyHostname(pretty string) error {
// 	pretty = strings.TrimSpace(pretty)
// 	if pretty == "" {
// 		return fmt.Errorf("pretty hostname is empty")
// 	}

// 	// --pretty sets PRETTY_HOSTNAME in /etc/machine-info
// 	cmd := exec.Command("hostnamectl", "set-hostname", "--pretty", pretty)
// 	out, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("hostnamectl failed: %w; output: %s", err, strings.TrimSpace(string(out)))
// 	}
// 	return nil
// }

// func main() {
// 	if err := SetPrettyHostname("My Server"); err != nil {
// 		panic(err)
// 	}
// }
