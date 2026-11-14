package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NeedsRoot reports whether this process must elevate to write to a device.
func NeedsRoot() bool {
	return os.Geteuid() != 0
}

// AuthenticateRoot validates sudo credentials. It must run with the terminal in
// normal (cooked) mode so sudo can read the password itself.
func AuthenticateRoot() error {
	out, err := exec.Command("sudo", "-v").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return fmt.Errorf("sudo authentication failed: %s", lastLines(text, 2))
		}
		return fmt.Errorf("sudo authentication failed")
	}
	return nil
}

// rootCommand builds a command that needs write access: directly when already
// root, through the just-validated sudo timestamp otherwise.
func rootCommand(name string, args ...string) *exec.Cmd {
	if NeedsRoot() {
		return exec.Command("sudo", append([]string{"-n", name}, args...)...)
	}
	return exec.Command(name, args...)
}
