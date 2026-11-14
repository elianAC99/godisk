// GoDisk is an inline helper to mount, unmount, eject, repair and format
// removable or external disks. It never lets you touch the system disk.
package main

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/elianAC99/godisk/actions"
	"github.com/elianAC99/godisk/disk"
	"github.com/elianAC99/godisk/ui"
)

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}
	if len(os.Args) > 2 {
		printUsage()
		os.Exit(2)
	}

	// Validate a named action before switching the terminal to raw mode.
	var action *actions.Action
	if len(os.Args) == 2 {
		found, ok := actions.Find(os.Args[1])
		if !ok {
			fmt.Fprintf(os.Stderr, "godisk: unknown action %q\n\n", os.Args[1])
			printUsage()
			os.Exit(2)
		}
		action = &found
	}

	term := &tty{fd: int(os.Stdin.Fd())}
	if err := term.makeRaw(); err != nil {
		fmt.Fprintf(os.Stderr, "godisk: %v\n", err)
		os.Exit(1)
	}
	defer term.restore()

	reader := bufio.NewReader(os.Stdin)

	for {
		// Without a named action, pick one inline first.
		if action == nil {
			picked, ok := chooseAction(reader)
			if !ok {
				term.restore()
				return
			}
			action = &picked
		}

		// Stay interactive: the disk list is redrawn after every run, so a
		// mounted or unmounted disk simply drops out of the refreshed list.
		if !pickDisk(reader, *action, term) {
			term.restore()
			return
		}
		action = nil
	}
}

// pickDisk shows the action's disk list, runs the chosen action and redraws the
// refreshed list. It returns false when the user cancels the whole program.
func pickDisk(reader *bufio.Reader, action actions.Action, term *tty) bool {
	for {
		disks, err := disk.List()
		if err != nil {
			ui.Fail(err.Error())
			return true
		}

		var candidates []disk.Disk
		for _, d := range disks {
			if action.Select(d) {
				candidates = append(candidates, d)
			}
		}
		rows := items(action, candidates)
		rows = append(rows, backItem())

		index, chosen := ui.Select(reader, "", header(action), rows)
		if !chosen {
			return false
		}
		if index == len(rows)-1 {
			return true
		}

		selected := candidates[index]
		detail, runErr := runAction(action, selected, term)
		if runErr != nil {
			ui.Fail(runErr.Error())
			continue
		}
		ui.Success(selected.Name() + " " + detail)
	}
}

// runAction executes the action. Root-only work asks sudo for the password with
// the terminal back in normal mode, then restores the selector. Elevating the
// whole program is not an option: udisksctl talks to the user's session bus.
func runAction(action actions.Action, d disk.Disk, term *tty) (string, error) {
	msg := action.Progress + " " + d.Name()

	// Erase only the previous action's message, never the whole terminal.
	ui.ClearMessages()

	if !action.Root || !actions.NeedsRoot() {
		return ui.Progress(msg, func() (string, error) {
			return action.Run(d)
		})
	}

	// sudo writes its own prompt, so remember where the message starts and
	// erase everything (status and prompt) once the action is over.
	ui.SaveCursor()
	ui.Status(msg)
	term.restore()
	detail, err := runRoot(action, d)
	// Without raw mode the selector cannot read keys, so that failure wins.
	if rawErr := term.makeRaw(); rawErr != nil {
		return "", rawErr
	}
	ui.RestoreCursor()
	return detail, err
}

// runRoot authenticates sudo on the terminal, then runs the action.
func runRoot(action actions.Action, d disk.Disk) (string, error) {
	if err := actions.AuthenticateRoot(); err != nil {
		return "", err
	}
	return action.Run(d)
}

// tty remembers the raw-mode state so root-only actions can hand the terminal
// to sudo for the password and then reclaim it.
type tty struct {
	fd    int
	state *term.State
}

func (t *tty) makeRaw() error {
	state, err := term.MakeRaw(t.fd)
	if err != nil {
		return err
	}
	t.state = state
	return nil
}

func (t *tty) restore() {
	if t.state == nil {
		return
	}
	if err := term.Restore(t.fd, t.state); err != nil {
		fmt.Fprintf(os.Stderr, "godisk: restoring the terminal: %v\n", err)
	}
}

// chooseAction shows the inline action menu used when godisk runs without an
// argument. It returns ok=false when the user cancels or picks Exit.
func chooseAction(reader *bufio.Reader) (actions.Action, bool) {
	registry := actions.All()

	entries := make([]ui.Item, 0, len(registry)+1)
	for _, action := range registry {
		entries = append(entries, ui.Item{Cells: []string{action.Title}, Desc: action.Description})
	}
	entries = append(entries, ui.Item{Cells: []string{"Exit"}, Desc: "Quit GoDisk"})

	index, chosen := ui.Select(reader, "GODISK ACTIONS", nil, entries)
	if !chosen || index == len(registry) {
		return actions.Action{}, false
	}
	return registry[index], true
}

// backItem is the last row of every disk list: it returns to the actions.
func backItem() ui.Item {
	return ui.Item{Cells: []string{"Back"}, Desc: "Return to the action list"}
}

// header returns the columns shown for the action's disk list.
func header(action actions.Action) []string {
	columns := []string{"NAME", "TYPE", "SIZE", "FSTYPE"}
	if action.Name == "unmount" {
		columns = append(columns, "MOUNTED AT")
	}
	return columns
}

// items converts the selectable disks into rendered rows.
func items(action actions.Action, disks []disk.Disk) []ui.Item {
	rows := make([]ui.Item, 0, len(disks))
	for _, d := range disks {
		cells := []string{d.Name(), d.Type(), d.Size, d.FSType}
		if action.Name == "unmount" {
			cells = append(cells, d.MountPoint)
		}
		rows = append(rows, ui.Item{Cells: cells, Desc: describe(d)})
	}
	return rows
}

func describe(d disk.Disk) string {
	if d.Mounted {
		return fmt.Sprintf("%s filesystem mounted at %s", d.FSType, d.MountPoint)
	}
	return fmt.Sprintf("%s filesystem on a %s disk", d.FSType, d.Type())
}

func printUsage() {
	fmt.Println("GoDisk: mount, unmount, eject, repair and format removable or external disks.")
	fmt.Println()
	fmt.Println("Usage: godisk [action]")
	fmt.Println()
	fmt.Println("Run without an action to pick one interactively.")
	fmt.Println()
	fmt.Println("Actions:")
	for _, action := range actions.All() {
		fmt.Printf("  %-9s %s\n", action.Name, action.Description)
	}
	fmt.Println()
	fmt.Println("Pick with the arrow keys, Enter to confirm, Esc to cancel.")
}
