package actions

import (
	"strings"

	"github.com/elianAC99/godisk/disk"
)

// Mount mounts a removable or external disk through udisks2, which places it
// under /run/media/<user>/<label> without requiring root.
var Mount = Action{
	Name:        "mount",
	Title:       "Mount",
	Progress:    "Mounting",
	Description: "Mount a removable or external disk",
	Select: func(d disk.Disk) bool {
		return !d.System && !d.Mounted && hasFilesystem(d)
	},
	Run: func(d disk.Disk) (string, error) {
		if err := refuseSystem(d); err != nil {
			return "", err
		}
		out, err := udisksctl("mount", d)
		if err != nil {
			return "", err
		}
		// "Mounted /dev/sdb1 at /run/media/user/LABEL."
		if at := strings.LastIndex(out, " at "); at >= 0 {
			return "mounted at " + strings.TrimSuffix(strings.TrimSpace(out[at+4:]), "."), nil
		}
		return "mounted", nil
	},
}
