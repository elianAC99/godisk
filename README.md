# GoDisk

GoDisk is a **Linux** command-line helper to mount, unmount, eject, repair and
format removable or external disks. It draws a small panel in place instead of
taking over the whole screen, always shows a friendly name (label, model, or
"Unnamed disk") instead of a device node, and never offers the disk that runs
the system. It only runs on Linux: it reads disks through `lsblk` and udisks2
and formats them with the Linux `mkfs` tools.

## Installation

There are no prebuilt binaries yet. With Go 1.27 or newer:

```sh
go install github.com/elianAC99/godisk@latest
```

## Usage

```sh
godisk              # pick an action interactively
godisk mount        # or name one: mount, unmount, eject, repair, format
godisk --help
```

## External dependencies

GoDisk calls the system's own disk tools. A normal Linux desktop already has
`lsblk`, `findmnt`, `fsck` and `eject` (util-linux), `udisksctl` (udisks2) and
`sudo`, so there is nothing to install for mounting, unmounting, ejecting or
repairing the usual filesystems.

To format, install only the package for the filesystem you need:

| Package | Provides | Filesystem |
| --- | --- | --- |
| `dosfstools` | `mkfs.vfat` | vfat / FAT32 |
| `exfatprogs` | `mkfs.exfat` | exFAT |
| `ntfs-3g` | `mkfs.ntfs` | NTFS |
| `e2fsprogs` | `mkfs.ext2/3/4` | ext2/3/4 (usually already installed) |
| `btrfs-progs` | `mkfs.btrfs` | btrfs |
| `xfsprogs` | `mkfs.xfs` | XFS |
| `f2fs-tools` | `mkfs.f2fs` | F2FS |

Repair runs the system `fsck`, which calls the checker of that filesystem
(`fsck.ext4` from e2fsprogs, `fsck.exfat` from exfatprogs, and so on), so the
same package covers repair and format.

## Notes

- Repair and format ask for your password through `sudo`; only that command is
  elevated, never the whole program.
- Format wipes a disk but keeps its label and filesystem: it does not rename or
  change the format.
- Repair and format only run on unmounted disks; mounting is done through
  udisks2 so it works without root.
