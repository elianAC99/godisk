package actions

import "github.com/elianAC99/godisk/disk"

// Unmount safely unmounts a removable or external disk through udisks2. The
// system disk is never offered and is refused here as a second guard.
var Unmount = Action{
	Name:        "unmount",
	Title:       "Unmount",
	Progress:    "Unmounting",
	Description: "Safely unmount a removable or external disk",
	Select: func(d disk.Disk) bool {
		return !d.System && d.Mounted && hasFilesystem(d)
	},
	Run: func(d disk.Disk) (string, error) {
		if err := refuseSystem(d); err != nil {
			return "", err
		}
		if _, err := udisksctl("unmount", d); err != nil {
			return "", err
		}
		return "unmounted", nil
	},
}
