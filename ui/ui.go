// Package ui renders an inline selector that replaces itself on each step:
// no full-screen TUI, no scrolling, no resized terminal.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ANSI styles and terminal control. Colours are standard SGR codes (not the
// bright 90-97 range) so every theme maps them to readable colours. Setting
// NO_COLOR disables every style, leaving plain text.
var (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

const (
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	clearLine  = "\x1b[2K"
)

func init() {
	if os.Getenv("NO_COLOR") != "" {
		reset, bold = "", ""
		red, green, yellow, cyan = "", "", "", ""
	}
}

// write emits terminal control sequences and text. A write error is not
// actionable in a command-line UI: the terminal is the only output, so if it
// is gone there is nowhere to report the failure.
func write(s string) {
	_, _ = os.Stdout.WriteString(s)
}

// Item is one selectable row.
type Item struct {
	// Cells are the column values, aligned across every row.
	Cells []string
	// Desc is shown under the list while the row holds the cursor.
	Desc string
}

// Select draws a selector in place and returns the chosen index. A non-empty
// title is printed as a header; a non-empty columns slice is printed as the
// aligned table header. The whole block is erased on Enter/Esc so the caller
// can draw the next step in the same place. ok is false when the user cancels.
func Select(r *bufio.Reader, title string, columns []string, items []Item) (int, bool) {
	if len(items) == 0 {
		return -1, false
	}

	cursor := 0
	widths := columnWidths(columns, items)

	write(hideCursor)
	defer write(showCursor)

	height := 0
	redraw := func() {
		if height > 0 {
			write(fmt.Sprintf("\x1b[%dA", height))
		}
		lines := buildLines(title, columns, items, widths, cursor)
		height = len(lines)
		for _, line := range lines {
			write(renderLine(line))
		}
	}

	for {
		redraw()

		switch ReadKey(r) {
		case "up":
			cursor = step(cursor, len(items), -1)
		case "down":
			cursor = step(cursor, len(items), 1)
		case "enter":
			clearBlock(height)
			return cursor, true
		case "esc", "quit":
			clearBlock(height)
			write(red + "Cancelled" + reset + "\r\n")
			return -1, false
		}
	}
}

// buildLines returns the exact lines of the selector block. The height is
// constant (optional title + optional column header + rows + one description
// line) so redraws never drift.
func buildLines(title string, columns []string, items []Item, widths []int, cursor int) []string {
	lines := make([]string, 0, len(items)+3)

	if title != "" {
		// Indent the title to the text column so it lines up with the labels
		// rather than with the circles.
		lines = append(lines, "  "+bold+title+reset)
	}

	if len(columns) > 0 {
		lines = append(lines, "  "+padCells(columns, widths))
	}

	for i, item := range items {
		if i == cursor {
			// Cyan characters only: no background, no reverse video.
			lines = append(lines, cyan+bold+"● "+padCells(item.Cells, widths)+reset)
			continue
		}
		lines = append(lines, "○ "+padCells(item.Cells, widths))
	}

	desc := ""
	if cursor >= 0 && cursor < len(items) {
		desc = items[cursor].Desc
	}
	lines = append(lines, "  "+desc)

	return lines
}

// renderLine erases the current line and only then writes its content. The
// order matters: ESC[2K must never follow the text or it wipes the line we just
// painted, leaving the whole block invisible on a real terminal.
func renderLine(line string) string {
	return clearLine + line + "\r\n"
}

// clearBlock erases the selector block and leaves the cursor at its first line.
func clearBlock(height int) {
	if height == 0 {
		return
	}
	write(fmt.Sprintf("\x1b[%dA", height))
	for i := 0; i < height; i++ {
		write(renderLine(""))
	}
	write(fmt.Sprintf("\x1b[%dA", height))
}

// messageHeight is how many message lines the last action left behind, so the
// next action erases exactly those lines and nothing else.
var messageHeight int

// ClearMessages erases only the previous action's message lines. The rest of
// the terminal (scrollback and the selector block) is left untouched.
func ClearMessages() {
	for i := 0; i < messageHeight; i++ {
		write("\x1b[A\r" + clearLine)
	}
	messageHeight = 0
}

// SaveCursor remembers the current position. RestoreCursor returns there and
// erases everything printed since, which is what work that takes over the
// terminal (sudo asking for the password) needs.
func SaveCursor() {
	write("\x1b[s")
}

// RestoreCursor goes back to the saved position and clears what is below it.
func RestoreCursor() {
	write("\x1b[u\x1b[0J")
	messageHeight = 0
}

// Status prints a cyan progress line without animation, for work that takes
// over the terminal (such as telling sudo to ask for the password).
func Status(msg string) {
	write(cyan + msg + reset + "\r\n")
}

// Success prints a green result line.
func Success(text string) {
	write(successLine(text))
	messageHeight = lineCount(text)
}

// Fail prints a red result line.
func Fail(text string) {
	write(failLine(text))
	messageHeight = lineCount(text)
}

// Warn prints a yellow informational line.
func Warn(text string) {
	write(warnLine(text))
	messageHeight = lineCount(text)
}

// lineCount is how many terminal lines a message occupies.
func lineCount(text string) int {
	return strings.Count(text, "\n") + 1
}

func successLine(text string) string { return green + text + reset + "\r\n" }
func failLine(text string) string    { return red + text + reset + "\r\n" }
func warnLine(text string) string    { return yellow + text + reset + "\r\n" }

// step moves the cursor by delta and wraps around the ends of the list.
func step(cursor, count, delta int) int {
	if count <= 0 {
		return 0
	}
	return ((cursor+delta)%count + count) % count
}

// columnWidths returns the display width of every column, computed over the
// header and every item so all rows line up.
func columnWidths(columns []string, items []Item) []int {
	count := len(columns)
	for _, item := range items {
		if len(item.Cells) > count {
			count = len(item.Cells)
		}
	}
	widths := make([]int, count)
	for i, cell := range columns {
		widths[i] = utf8.RuneCountInString(cell)
	}
	for _, item := range items {
		for i, cell := range item.Cells {
			if w := utf8.RuneCountInString(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// padCells joins cells with a two-space gap, padding each to its width.
func padCells(cells []string, widths []int) string {
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		width := 0
		if i < len(widths) {
			width = widths[i]
		}
		b.WriteString(padRight(cell, width))
	}
	return b.String()
}

// padRight pads s up to width with spaces, counting runes (not bytes) so
// multi-byte glyphs do not break the alignment.
func padRight(s string, width int) string {
	missing := width - utf8.RuneCountInString(s)
	if missing <= 0 {
		return s
	}
	return s + strings.Repeat(" ", missing)
}
