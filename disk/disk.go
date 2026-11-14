// Package disk lists mountable filesystems and classifies which of them are
// part of the running system, so that callers can refuse to touch those.
package disk

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Disk is a single mountable filesystem entry as reported by lsblk.
type Disk struct {
	// Path is the device node (e.g. /dev/sdb1). It is used for commands but
	// must never be shown to the user as the disk name.
	Path string
	// DrivePath is the device node of the whole disk that holds this entry
	// (/dev/sdb for /dev/sdb1, and /dev/sdb for the disk itself).
	DrivePath  string
	Label      string
	Model      string
	FSType     string
	Size       string
	Tran       string
	Removable  bool
	Mounted    bool
	MountPoint string
	// System is true when the disk hosts the running system (root filesystem
	// or any non-removable mount point). Such disks must never be unmounted
	// or repaired.
	System bool
}

// Name returns the human readable name of the disk: its volume label when it
// has one, otherwise the hardware model, otherwise a generic placeholder.
func (d Disk) Name() string {
	if label := strings.TrimSpace(d.Label); label != "" {
		return label
	}
	if model := strings.TrimSpace(d.Model); model != "" {
		return model
	}
	return "Unnamed disk"
}

// Type returns the transport of the disk (usb, sata, nvme...).
func (d Disk) Type() string {
	if d.Tran != "" {
		return d.Tran
	}
	return "disk"
}

// List returns the mountable filesystems currently attached to the machine.
func List() ([]Disk, error) {
	out, err := exec.Command(
		"lsblk", "-J", "-e7",
		"-o", "NAME,PATH,TYPE,SIZE,TRAN,RM,MODEL,LABEL,FSTYPE,MOUNTPOINT,PKNAME",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("running lsblk: %w", err)
	}
	return parseLsblk(out, rootDiskName())
}

// rootDiskName returns the kernel name (sda, nvme0n1...) of the disk that
// backs the root filesystem, or "" when it cannot be resolved.
func rootDiskName() string {
	out, err := exec.Command("findmnt", "-no", "SOURCE", "/").Output()
	if err != nil {
		return ""
	}
	source := strings.TrimSpace(string(out))
	// btrfs subvolumes are reported as /dev/sda2[/@].
	if i := strings.IndexByte(source, '['); i >= 0 {
		source = source[:i]
	}
	if source == "" {
		return ""
	}
	out, err = exec.Command("lsblk", "-no", "PKNAME", source).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type blockDevice struct {
	Name       string        `json:"name"`
	Path       string        `json:"path"`
	Type       string        `json:"type"`
	Size       string        `json:"size"`
	Tran       string        `json:"tran"`
	RM         bool          `json:"rm"`
	Model      string        `json:"model"`
	Label      string        `json:"label"`
	FSType     string        `json:"fstype"`
	MountPoint string        `json:"mountpoint"`
	PKName     string        `json:"pkname"`
	Children   []blockDevice `json:"children"`
}

func parseLsblk(data []byte, rootDisk string) ([]Disk, error) {
	var doc struct {
		Blockdevices []blockDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing lsblk output: %w", err)
	}

	var disks []Disk
	for _, top := range doc.Blockdevices {
		if top.Type == "rom" || top.Type == "loop" {
			continue
		}
		system := top.Name == rootDisk || hasSystemMount(top)
		if d, ok := entry(top, top, system); ok {
			disks = append(disks, d)
		}
		for _, child := range top.Children {
			if child.Type != "part" && child.Type != "disk" {
				continue
			}
			if d, ok := entry(top, child, system); ok {
				disks = append(disks, d)
			}
		}
	}
	return disks, nil
}

// entry builds a Disk for dev, inheriting the hardware facts of its parent
// disk. It reports false when dev does not hold a mountable filesystem.
func entry(parent, dev blockDevice, system bool) (Disk, bool) {
	if !hasFilesystem(dev.FSType) {
		return Disk{}, false
	}
	return Disk{
		Path:       dev.Path,
		DrivePath:  parent.Path,
		Label:      dev.Label,
		Model:      parent.Model,
		FSType:     dev.FSType,
		Size:       dev.Size,
		Tran:       parent.Tran,
		Removable:  parent.RM,
		Mounted:    dev.MountPoint != "" && dev.MountPoint != "[SWAP]",
		MountPoint: dev.MountPoint,
		System:     system,
	}, true
}

func hasFilesystem(fsType string) bool {
	return fsType != "" && fsType != "swap"
}

// hasSystemMount reports whether dev, or any of its children, is mounted
// somewhere that does not look like a user-mounted removable location.
func hasSystemMount(dev blockDevice) bool {
	if dev.MountPoint != "" && !isExternalMount(dev.MountPoint) {
		return true
	}
	for _, child := range dev.Children {
		if hasSystemMount(child) {
			return true
		}
	}
	return false
}

// isExternalMount reports whether mountPoint is a place where removable or
// external media live. Anything else is treated as part of the system.
func isExternalMount(mountPoint string) bool {
	if mountPoint == "" || mountPoint == "[SWAP]" {
		return false
	}
	for _, prefix := range []string{"/media/", "/run/media/", "/mnt/"} {
		if strings.HasPrefix(mountPoint, prefix) {
			return true
		}
	}
	return false
}
