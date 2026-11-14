package actions

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/elianAC99/godisk/disk"
)

// Repair runs fsck in automatic mode on an unmounted removable or external
// disk. It needs root privileges.
var Repair = Action{
	Name:        "repair",
	Title:       "Repair",
	Progress:    "Repairing",
	Description: "Check and repair a filesystem",
	Root:        true,
	Select: func(d disk.Disk) bool {
		return !d.System && !d.Mounted && hasFilesystem(d)
	},
	Run: func(d disk.Disk) (string, error) {
		if err := refuseSystem(d); err != nil {
			return "", err
		}
		// Repairing a mounted filesystem can corrupt it, so re-check here:
		// the disk may have been mounted after the list was drawn.
		if d.Mounted {
			return "", fmt.Errorf("refusing to repair %q: it is mounted", d.Name())
		}

		var output bytes.Buffer
		cmd := rootCommand("fsck", "-y", d.Path)
		cmd.Stdout = &output
		cmd.Stderr = &output

		err := cmd.Run()
		if err == nil {
			return "no errors found", nil
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// sudo's own failures would otherwise look like fsck's exit codes.
			if strings.Contains(output.String(), "sudo:") {
				return "", fmt.Errorf("%s", lastLines(strings.TrimSpace(output.String()), 2))
			}
			// A permission failure can also surface as exit 4 (errors left),
			// so report the real cause first: fsck could not write.
			if needsRoot(output.String()) {
				return "", fmt.Errorf("repair needs root privileges: run godisk with sudo")
			}
			switch exitErr.ExitCode() {
			case 1:
				return "errors fixed", nil
			case 2:
				return "errors fixed (reboot recommended)", nil
			case 4:
				return "", fmt.Errorf("errors remain after repair: %s", lastLines(strings.TrimSpace(output.String()), 3))
			}
		}

		// fsck needs write access; without root it exits 8 with a permission error.
		if needsRoot(output.String()) {
			return "", fmt.Errorf("repair needs root privileges: run godisk with sudo")
		}

		detail := strings.TrimSpace(output.String())
		if detail == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", lastLines(detail, 3))
	},
}

// needsRoot reports whether fsck failed only because it could not write to the
// device, which usually means it was started without root privileges.
func needsRoot(output string) bool {
	lower := strings.ToLower(output)
	for _, hint := range []string{
		"permission denied",
		"operation not permitted",
		"must have r/w access",
		"read-only file system",
	} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// lastLines keeps the last n lines of output so error notices stay readable.
func lastLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
