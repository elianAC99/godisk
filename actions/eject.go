package actions

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/elianAC99/godisk/disk"
)

// Eject releases the hardware behind a disk. Every mounted filesystem on the
// drive is unmounted first, then USB devices are powered off (safe removal)
// while other disks are ejected.
var Eject = Action{
	Name:        "eject",
	Title:       "Eject",
	Progress:    "Ejecting",
	Description: "Unmount and eject a disk or power off a USB device",
	Select: func(d disk.Disk) bool {
		return !d.System && hasFilesystem(d)
	},
	Run: func(d disk.Disk) (string, error) {
		if err := refuseSystem(d); err != nil {
			return "", err
		}

		drive := d.DrivePath
		if drive == "" {
			drive = d.Path
		}

		// Unmount every filesystem living on this drive before releasing it.
		if err := unmountDrive(drive, d); err != nil {
			return "", err
		}

		if d.Tran == "usb" {
			if _, err := udisksctlOn("power-off", drive); err != nil {
				return "", err
			}
			return "powered off", nil
		}

		out, err := exec.Command("eject", drive).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil {
			if text == "" {
				return "", err
			}
			return "", fmt.Errorf("%s", text)
		}
		return "ejected", nil
	},
}

// unmountDrive unmounts every mounted filesystem on drive, falling back to the
// selected disk when the current list cannot be read.
func unmountDrive(drive string, selected disk.Disk) error {
	listed, err := disk.List()
	if err != nil {
		if selected.Mounted {
			_, err = udisksctl("unmount", selected)
		}
		return err
	}

	for _, d := range listed {
		if d.DrivePath != drive || !d.Mounted {
			continue
		}
		if _, err := udisksctl("unmount", d); err != nil {
			return err
		}
	}
	return nil
}
