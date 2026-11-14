package actions

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/elianAC99/godisk/disk"
)

// Format erases a disk and creates the same filesystem again with the same
// label. For now it never renames nor changes the format: it just wipes.
var Format = Action{
	Name:        "format",
	Title:       "Format",
	Progress:    "Formatting",
	Description: "Erase a disk keeping its label and filesystem",
	Root:        true,
	Select: func(d disk.Disk) bool {
		return !d.System && !d.Mounted && hasFilesystem(d)
	},
	Run: func(d disk.Disk) (string, error) {
		if err := refuseSystem(d); err != nil {
			return "", err
		}
		// Wiping a mounted filesystem destroys data that is in use.
		if d.Mounted {
			return "", fmt.Errorf("refusing to format %q: it is mounted", d.Name())
		}

		tool, args, err := mkfsCommand(d.FSType, d.Label)
		if err != nil {
			return "", err
		}

		var output bytes.Buffer
		cmd := rootCommand(tool, append(args, d.Path)...)
		cmd.Stdout = &output
		cmd.Stderr = &output

		if err := cmd.Run(); err != nil {
			if strings.Contains(output.String(), "sudo:") {
				return "", fmt.Errorf("%s", lastLines(strings.TrimSpace(output.String()), 2))
			}
			detail := strings.TrimSpace(output.String())
			if detail == "" {
				return "", err
			}
			return "", fmt.Errorf("%s", lastLines(detail, 3))
		}
		return "wiped", nil
	},
}

// mkfsCommand maps the filesystem already on the disk to its mkfs tool, using
// the flags that keep the label. An empty label is left unset instead of being
// written as an empty name.
func mkfsCommand(fstype, label string) (string, []string, error) {
	switch strings.ToLower(fstype) {
	case "vfat", "fat", "fat16", "fat32":
		return "mkfs.vfat", labelArgs("-n", label), nil
	case "exfat":
		return "mkfs.exfat", labelArgs("-n", label), nil
	case "ntfs":
		return "mkfs.ntfs", labelArgs("-L", label, "-f"), nil
	case "ext2", "ext3", "ext4":
		return "mkfs." + strings.ToLower(fstype), labelArgs("-L", label, "-F"), nil
	case "btrfs":
		return "mkfs.btrfs", labelArgs("-L", label, "-f"), nil
	case "xfs":
		return "mkfs.xfs", labelArgs("-L", label, "-f"), nil
	case "f2fs":
		return "mkfs.f2fs", labelArgs("-l", label, "-f"), nil
	}
	return "", nil, fmt.Errorf("format does not support %s yet", fstype)
}

// labelArgs returns the label flag plus the unavoidable extra flags; the label
// pair comes first so tests and error output read as "<flag> <label> ...".
func labelArgs(flag, label string, extra ...string) []string {
	args := make([]string, 0, len(extra)+2)
	if label != "" {
		args = append(args, flag, label)
	}
	return append(args, extra...)
}
