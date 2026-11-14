package disk

import "testing"

// fixture mirrors real `lsblk -J -e7` output, plus a USB stick, an external
// drive mounted under /run/media, a whole-disk filesystem and an unlabeled
// disk (model fallback).
const fixture = `{
  "blockdevices": [
    {
      "name": "sda", "path": "/dev/sda", "type": "disk", "size": "119.2G",
      "tran": "sata", "rm": false, "model": "Patriot P210 128GB",
      "label": null, "fstype": null, "mountpoint": null, "pkname": null,
      "children": [
        { "name": "sda1", "path": "/dev/sda1", "type": "part", "size": "1G",
          "tran": null, "rm": false, "model": null, "label": null,
          "fstype": "vfat", "mountpoint": "/boot", "pkname": "sda" },
        { "name": "sda2", "path": "/dev/sda2", "type": "part", "size": "118.2G",
          "tran": null, "rm": false, "model": null, "label": null,
          "fstype": "btrfs", "mountpoint": "/var/log", "pkname": "sda" }
      ]
    },
    {
      "name": "sdb", "path": "/dev/sdb", "type": "disk", "size": "28.7G",
      "tran": "usb", "rm": true, "model": "USB Flash Drive",
      "label": null, "fstype": null, "mountpoint": null, "pkname": null,
      "children": [
        { "name": "sdb1", "path": "/dev/sdb1", "type": "part", "size": "28.7G",
          "tran": null, "rm": true, "model": null, "label": "KINGSTON_1",
          "fstype": "ext4", "mountpoint": null, "pkname": "sdb" }
      ]
    },
    {
      "name": "sdc", "path": "/dev/sdc", "type": "disk", "size": "1.8T",
      "tran": "usb", "rm": true, "model": "My Passport 2626",
      "label": null, "fstype": null, "mountpoint": null, "pkname": null,
      "children": [
        { "name": "sdc1", "path": "/dev/sdc1", "type": "part", "size": "1.8T",
          "tran": null, "rm": true, "model": null, "label": "BACKUP",
          "fstype": "exfat", "mountpoint": "/run/media/elianac/BACKUP", "pkname": "sdc" }
      ]
    },
    { "name": "sdd", "path": "/dev/sdd", "type": "disk", "size": "57.3G",
      "tran": "usb", "rm": true, "model": null, "label": "SANDISK",
      "fstype": "exfat", "mountpoint": null, "pkname": null },
    { "name": "sdf", "path": "/dev/sdf", "type": "disk", "size": "14.6G",
      "tran": "usb", "rm": true, "model": "Generic USB", "label": null,
      "fstype": "vfat", "mountpoint": null, "pkname": null },
    { "name": "sr0", "path": "/dev/sr0", "type": "rom", "size": "1024M",
      "tran": "sata", "rm": true, "model": "hp DVDRW DU8A6SH",
      "label": null, "fstype": null, "mountpoint": null, "pkname": null },
    { "name": "zram0", "path": "/dev/zram0", "type": "disk", "size": "4G",
      "tran": null, "rm": false, "model": null, "label": "zram0",
      "fstype": "swap", "mountpoint": "[SWAP]", "pkname": null },
    { "name": "loop0", "path": "/dev/loop0", "type": "loop", "size": "64M",
      "tran": null, "rm": false, "model": null, "label": null,
      "fstype": "squashfs", "mountpoint": "/snap/core/1", "pkname": null }
  ]
}`

func index(t *testing.T, rootDisk string) map[string]Disk {
	t.Helper()
	got, err := parseLsblk([]byte(fixture), rootDisk)
	if err != nil {
		t.Fatalf("parseLsblk() error = %v", err)
	}
	byPath := make(map[string]Disk, len(got))
	for _, d := range got {
		byPath[d.Path] = d
	}
	return byPath
}

func TestParseListsOnlyMountableFilesystems(t *testing.T) {
	byPath := index(t, "sda")

	for _, want := range []string{"/dev/sda1", "/dev/sda2", "/dev/sdb1", "/dev/sdc1", "/dev/sdd", "/dev/sdf"} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("missing entry %q", want)
		}
	}
	for _, unwanted := range []string{"/dev/sda", "/dev/sr0", "/dev/zram0", "/dev/loop0"} {
		if _, ok := byPath[unwanted]; ok {
			t.Errorf("entry %q must not be listed", unwanted)
		}
	}
}

func TestParseRecordsTheDriveOfEachEntry(t *testing.T) {
	byPath := index(t, "sda")

	tests := map[string]string{
		"/dev/sda1": "/dev/sda",
		"/dev/sda2": "/dev/sda",
		"/dev/sdb1": "/dev/sdb",
		"/dev/sdd":  "/dev/sdd",
	}

	for path, want := range tests {
		if got := byPath[path].DrivePath; got != want {
			t.Errorf("%s DrivePath = %q, want %q", path, got, want)
		}
	}
}

func TestParseProtectsTheSystemDisk(t *testing.T) {
	byPath := index(t, "sda")

	if !byPath["/dev/sda1"].System {
		t.Error("/dev/sda1 is on the root disk and must be protected")
	}
	if !byPath["/dev/sda2"].System {
		t.Error("/dev/sda2 is on the root disk and must be protected")
	}
	for _, path := range []string{"/dev/sdb1", "/dev/sdc1", "/dev/sdd", "/dev/sdf"} {
		if byPath[path].System {
			t.Errorf("%s is removable/external and must not be protected", path)
		}
	}
}

func TestParseProtectsRootDiskEvenUnderAllowedMountpoints(t *testing.T) {
	byPath := index(t, "sdb")

	if !byPath["/dev/sdb1"].System {
		t.Error("the root disk must be protected regardless of where it is mounted")
	}
	if byPath["/dev/sdc1"].System {
		t.Error("an external disk must stay usable when another disk is the root")
	}
}

func TestParseDerivesHumanReadableNames(t *testing.T) {
	byPath := index(t, "sda")

	if got := byPath["/dev/sdb1"].Name(); got != "KINGSTON_1" {
		t.Errorf("Name() = %q, want the volume label %q", got, "KINGSTON_1")
	}
	if got := byPath["/dev/sdd"].Name(); got != "SANDISK" {
		t.Errorf("Name() = %q, want %q", got, "SANDISK")
	}
	if got := byPath["/dev/sdf"].Name(); got != "Generic USB" {
		t.Errorf("Name() = %q, want the model fallback %q", got, "Generic USB")
	}
	if got := byPath["/dev/sdb1"].Type(); got != "usb" {
		t.Errorf("Type() = %q, want %q", got, "usb")
	}
	if got := byPath["/dev/sdc1"].MountPoint; got != "/run/media/elianac/BACKUP" {
		t.Errorf("MountPoint = %q", got)
	}
	if !byPath["/dev/sdc1"].Mounted {
		t.Error("/dev/sdc1 is mounted")
	}
	if byPath["/dev/sdb1"].Mounted {
		t.Error("/dev/sdb1 is not mounted")
	}
}

func TestNameAndTypeFallbacks(t *testing.T) {
	var d Disk
	if got := d.Name(); got != "Unnamed disk" {
		t.Errorf("Name() = %q, want %q", got, "Unnamed disk")
	}
	if got := d.Type(); got != "disk" {
		t.Errorf("Type() = %q, want %q", got, "disk")
	}
}

func TestIsExternalMount(t *testing.T) {
	cases := map[string]bool{
		"":                          false,
		"/":                         false,
		"/boot":                     false,
		"/var/log":                  false,
		"/home":                     false,
		"[SWAP]":                    false,
		"/media/usb":                true,
		"/run/media/elianac/BACKUP": true,
		"/mnt/backup":               true,
	}
	for mountPoint, want := range cases {
		if got := isExternalMount(mountPoint); got != want {
			t.Errorf("isExternalMount(%q) = %v, want %v", mountPoint, got, want)
		}
	}
}
