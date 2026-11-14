package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elianAC99/godisk/disk"
)

func TestSelectListsTheRightDisksForEachAction(t *testing.T) {
	system := disk.Disk{Path: "/dev/sda2", FSType: "btrfs", Mounted: true, System: true}
	mounted := disk.Disk{Path: "/dev/sdb1", FSType: "ext4", Mounted: true}
	unmounted := disk.Disk{Path: "/dev/sdc1", FSType: "ext4"}
	noFilesystem := disk.Disk{Path: "/dev/sdd"}

	cases := []struct {
		name   string
		action Action
		want   map[string]bool
	}{
		{
			name:   "mount",
			action: Mount,
			want:   map[string]bool{system.Path: false, mounted.Path: false, unmounted.Path: true, noFilesystem.Path: false},
		},
		{
			name:   "unmount",
			action: Unmount,
			want:   map[string]bool{system.Path: false, mounted.Path: true, unmounted.Path: false, noFilesystem.Path: false},
		},
		{
			name:   "repair",
			action: Repair,
			want:   map[string]bool{system.Path: false, mounted.Path: false, unmounted.Path: true, noFilesystem.Path: false},
		},
		{
			name:   "eject",
			action: Eject,
			want:   map[string]bool{system.Path: false, mounted.Path: true, unmounted.Path: true, noFilesystem.Path: false},
		},
		{
			name:   "format",
			action: Format,
			want:   map[string]bool{system.Path: false, mounted.Path: false, unmounted.Path: true, noFilesystem.Path: false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, d := range []disk.Disk{system, mounted, unmounted, noFilesystem} {
				if got := tc.action.Select(d); got != tc.want[d.Path] {
					t.Errorf("%s Select(%s) = %v, want %v", tc.name, d.Path, got, tc.want[d.Path])
				}
			}
		})
	}
}

func TestActionsRefuseTheSystemDisk(t *testing.T) {
	system := disk.Disk{Path: "/dev/sda2", FSType: "btrfs", Mounted: true, System: true}

	for _, action := range All() {
		if _, err := action.Run(system); err == nil || !strings.Contains(err.Error(), "system") {
			t.Errorf("%s.Run(system disk) error = %v, want a refusal mentioning the system disk", action.Name, err)
		}
	}
}

func TestAllReturnsEveryAction(t *testing.T) {
	got := make([]string, 0, len(All()))
	for _, action := range All() {
		got = append(got, action.Name)
	}
	want := []string{"mount", "unmount", "eject", "repair", "format"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("All() names = %v, want %v", got, want)
	}
}

// fakeFsck puts a script named fsck first in PATH that exits with the code in
// FAKE_FSCK_EXIT, prints FAKE_FSCK_OUT and records each call in FAKE_FSCK_LOG.
func fakeFsck(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"printf '%s\\n' \"$FAKE_FSCK_OUT\"\n" +
		"exit \"$FAKE_FSCK_EXIT\"\n"
	if err := os.WriteFile(filepath.Join(dir, "fsck"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fsck: %v", err)
	}
	// fsck runs through "sudo -n" when the tests are not root; strip the flag.
	sudo := "#!/bin/sh\n" +
		"[ \"$1\" = \"-n\" ] && shift\n" +
		"exec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0o755); err != nil {
		t.Fatalf("write fake sudo: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_FSCK_LOG", log)
}

func TestRepairMapsFsckExitCodes(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		output  string
		want    string
		wantErr string
	}{
		{name: "clean filesystem", code: "0", want: "no errors found"},
		{name: "errors fixed", code: "1", want: "errors fixed"},
		{name: "errors fixed, reboot needed", code: "2", want: "errors fixed (reboot recommended)"},
		{name: "errors left uncorrected", code: "4", output: "UNEXPECTED INCONSISTENCY", wantErr: "errors remain"},
		{name: "no root privileges", code: "8", output: "fsck: /dev/sdc1: Permission denied", wantErr: "root privileges"},
		{name: "permission failure reported as uncorrected errors", code: "4", output: "e2fsck: Permission denied while trying to open /dev/sdc1", wantErr: "root privileges"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeFsck(t)
			t.Setenv("FAKE_FSCK_EXIT", tt.code)
			t.Setenv("FAKE_FSCK_OUT", tt.output)

			got, err := Repair.Run(disk.Disk{Path: "/dev/sdc1", FSType: "ext4"})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("detail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepairRefusesAMountedDiskWithoutRunningFsck(t *testing.T) {
	fakeFsck(t)
	t.Setenv("FAKE_FSCK_EXIT", "0")
	t.Setenv("FAKE_FSCK_OUT", "")

	_, err := Repair.Run(disk.Disk{Path: "/dev/sdc1", FSType: "ext4", Mounted: true})
	if err == nil || !strings.Contains(err.Error(), "mounted") {
		t.Fatalf("error = %v, want a refusal mentioning that it is mounted", err)
	}
	if _, statErr := os.Stat(os.Getenv("FAKE_FSCK_LOG")); statErr == nil {
		t.Error("fsck must not run on a mounted disk")
	}
}

// fakeEjectTools fakes udisksctl, eject, lsblk and findmnt for one drive with
// two mounted partitions, recording every command in a log file.
func fakeEjectTools(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}

	write("udisksctl", "printf 'udisksctl %s\\n' \"$*\" >> "+log+"\nexit 0\n")
	write("eject", "printf 'eject %s\\n' \"$*\" >> "+log+"\nexit 0\n")
	write("findmnt", "echo /dev/sda2\nexit 0\n")
	write("lsblk", `case "$*" in
  *-J*) cat <<'JSON'
{"blockdevices":[
 {"name":"sdb","path":"/dev/sdb","type":"disk","tran":"usb","rm":true,"model":"Stick",
  "children":[
   {"name":"sdb1","path":"/dev/sdb1","type":"part","size":"8G","fstype":"ext4","mountpoint":"/run/media/u/A","pkname":"sdb"},
   {"name":"sdb2","path":"/dev/sdb2","type":"part","size":"8G","fstype":"vfat","mountpoint":"/run/media/u/B","pkname":"sdb"}
  ]}
]}
JSON
    ;;
  *) echo "" ;;
esac
`)

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestEjectUnmountsEveryPartitionThenPowersOffUSB(t *testing.T) {
	log := fakeEjectTools(t)

	detail, err := Eject.Run(disk.Disk{Path: "/dev/sdb1", DrivePath: "/dev/sdb", FSType: "ext4", Tran: "usb"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != "powered off" {
		t.Errorf("detail = %q, want %q", detail, "powered off")
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"udisksctl unmount -b /dev/sdb1",
		"udisksctl unmount -b /dev/sdb2",
		"udisksctl power-off -b /dev/sdb",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("commands = %q, want %q", got, want)
	}
}

func TestEjectUsesEjectForNonUSBDisks(t *testing.T) {
	log := fakeEjectTools(t)

	detail, err := Eject.Run(disk.Disk{Path: "/dev/sdb1", DrivePath: "/dev/sdb", FSType: "ext4", Tran: "sata"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != "ejected" {
		t.Errorf("detail = %q, want %q", detail, "ejected")
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	if !strings.Contains(string(raw), "eject /dev/sdb") {
		t.Errorf("commands = %q, want an eject of the whole drive", raw)
	}
	if !strings.Contains(string(raw), "udisksctl unmount") {
		t.Errorf("commands = %q, want the mounted partitions unmounted first", raw)
	}
}

func TestRootCommandUsesCachedSudoWhenNotRoot(t *testing.T) {
	cmd := rootCommand("fsck", "-y", "/dev/sdc1")

	want := []string{"fsck", "-y", "/dev/sdc1"}
	if NeedsRoot() {
		want = append([]string{"sudo", "-n"}, want...)
	}
	if strings.Join(cmd.Args, " ") != strings.Join(want, " ") {
		t.Errorf("command = %v, want %v", cmd.Args, want)
	}
}

func TestRepairReportsSudoFailureInsteadOfFixing(t *testing.T) {
	if !NeedsRoot() {
		t.Skip("already running as root: sudo is not used")
	}

	dir := t.TempDir()
	sudo := "#!/bin/sh\necho 'sudo: a password is required' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0o755); err != nil {
		t.Fatalf("write fake sudo: %v", err)
	}
	fsck := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "fsck"), []byte(fsck), 0o755); err != nil {
		t.Fatalf("write fake fsck: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := Repair.Run(disk.Disk{Path: "/dev/sdc1"})
	if err == nil {
		t.Fatal("expected a sudo failure, got success")
	}
	if !strings.Contains(err.Error(), "password is required") {
		t.Errorf("error = %q, want it to report the sudo failure", err)
	}
}

func TestFormatKeepsTheFilesystemAndLabel(t *testing.T) {
	cases := []struct {
		fstype string
		tool   string
		args   string
	}{
		{fstype: "vfat", tool: "mkfs.vfat", args: "-n KINGSTON"},
		{fstype: "fat32", tool: "mkfs.vfat", args: "-n KINGSTON"},
		{fstype: "exfat", tool: "mkfs.exfat", args: "-n KINGSTON"},
		{fstype: "ntfs", tool: "mkfs.ntfs", args: "-L KINGSTON -f"},
		{fstype: "ext4", tool: "mkfs.ext4", args: "-L KINGSTON -F"},
		{fstype: "ext3", tool: "mkfs.ext3", args: "-L KINGSTON -F"},
		{fstype: "btrfs", tool: "mkfs.btrfs", args: "-L KINGSTON -f"},
		{fstype: "xfs", tool: "mkfs.xfs", args: "-L KINGSTON -f"},
		{fstype: "f2fs", tool: "mkfs.f2fs", args: "-l KINGSTON -f"},
		{fstype: "vfat", tool: "mkfs.vfat", args: "-n KINGSTON"},
	}

	for _, tc := range cases {
		tool, args, err := mkfsCommand(tc.fstype, "KINGSTON")
		if err != nil {
			t.Fatalf("mkfsCommand(%q) error = %v", tc.fstype, err)
		}
		if tool != tc.tool || strings.Join(args, " ") != tc.args {
			t.Errorf("mkfsCommand(%q) = %s %v, want %s %s", tc.fstype, tool, args, tc.tool, tc.args)
		}
	}
}

func TestFormatLeavesAnEmptyLabelUnset(t *testing.T) {
	_, args, err := mkfsCommand("vfat", "")
	if err != nil {
		t.Fatalf("mkfsCommand error = %v", err)
	}
	if len(args) != 0 {
		t.Errorf("args = %v, want no label flags for an empty label", args)
	}
}

func TestFormatRejectsAFilesystemItCannotRebuild(t *testing.T) {
	_, _, err := mkfsCommand("zfs", "POOL")
	if err == nil || !strings.Contains(err.Error(), "does not support zfs") {
		t.Errorf("mkfsCommand(zfs) error = %v, want an unsupported-filesystem message", err)
	}
}

func TestFormatRunsMkfsWithTheSameLabel(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	// printf, not echo: dash's echo eats a leading -n as its own option.
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "mkfs.vfat"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake mkfs.vfat: %v", err)
	}
	sudo := "#!/bin/sh\n[ \"$1\" = \"-n\" ] && shift\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0o755); err != nil {
		t.Fatalf("write fake sudo: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	detail, err := Format.Run(disk.Disk{Path: "/dev/sdc1", FSType: "vfat", Label: "KINGSTON"})
	if err != nil {
		t.Fatalf("Format.Run error = %v", err)
	}
	if detail != "wiped" {
		t.Errorf("detail = %q, want %q", detail, "wiped")
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	if got := strings.TrimSpace(string(calls)); got != "-n KINGSTON /dev/sdc1" {
		t.Errorf("mkfs called with %q, want %q", got, "-n KINGSTON /dev/sdc1")
	}
}

func TestFormatRefusesAMountedDiskWithoutRunningMkfs(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "mkfs.ext4"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake mkfs.ext4: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	d := disk.Disk{Path: "/dev/sdc1", FSType: "ext4", Label: "BACKUP", Mounted: true}
	if _, err := Format.Run(d); err == nil || !strings.Contains(err.Error(), "mounted") {
		t.Fatalf("Format.Run(mounted) error = %v, want a mounted refusal", err)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("mkfs ran on a mounted disk")
	}
}
