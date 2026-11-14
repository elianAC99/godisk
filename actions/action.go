// Package actions holds every operation GoDisk can perform on a disk. Adding
// a new action means adding one file here and one line in All.
package actions

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/elianAC99/godisk/disk"
)

// Action is one operation GoDisk can run on a disk.
type Action struct {
	// Name is the stable identifier (mount, unmount, repair).
	Name string
	// Title is the label shown in the main menu.
	Title string
	// Progress is the verb shown while the action runs ("Mounting ...").
	Progress string
	// Description is shown under the selected menu entry.
	Description string
	// Root marks actions that need root privileges to write to the device.
	Root bool
	// Select reports whether the action can be applied to d.
	Select func(d disk.Disk) bool
	// Run performs the action and returns a short success detail.
	Run func(d disk.Disk) (string, error)
}

// All returns the actions exposed in the UI, in display order.
func All() []Action {
	return []Action{Mount, Unmount, Eject, Repair, Format}
}

// Find returns the action registered under name.
func Find(name string) (Action, bool) {
	for _, action := range All() {
		if action.Name == name {
			return action, true
		}
	}
	return Action{}, false
}

// refuseSystem blocks every mutating action on the disk that hosts the running
// system. This is the last line of defence: listings already hide those disks.
func refuseSystem(d disk.Disk) error {
	if d.System {
		return fmt.Errorf("refusing to operate on the system disk %q", d.Name())
	}
	return nil
}

func hasFilesystem(d disk.Disk) bool {
	return d.FSType != "" && d.FSType != "swap"
}

// udisksctl runs udisksctl for the given verb against d and returns its
// combined output on failure so the user sees the real reason.
func udisksctl(verb string, d disk.Disk) (string, error) {
	return udisksctlOn(verb, d.Path)
}

// udisksctlOn runs udisksctl for the given verb against an explicit device.
func udisksctlOn(verb, path string) (string, error) {
	out, err := exec.Command("udisksctl", verb, "-b", path).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}
