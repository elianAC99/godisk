package ui

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func TestColumnWidthsAlignsHeaderAndRows(t *testing.T) {
	columns := []string{"NAME", "TYPE", "SIZE"}
	items := []Item{
		{Cells: []string{"KINGSTON_1", "usb", "28.7G"}},
		{Cells: []string{"DISCO", "sata", "1.8T"}},
	}

	got := columnWidths(columns, items)
	want := []int{10, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("columnWidths()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestBuildLinesAlignsCellsUnderTheColumnHeader(t *testing.T) {
	columns := []string{"NAME", "TYPE", "SIZE"}
	items := []Item{
		{Cells: []string{"KINGSTON_1", "usb", "28.7G"}, Desc: "first"},
		{Cells: []string{"DISCO", "sata", "1.8T"}, Desc: "second"},
	}
	widths := columnWidths(columns, items)

	for cursor := range items {
		lines := buildLines("", columns, items, widths, cursor)
		headerLine := stripANSI(lines[0])
		wantColumn := utf8.RuneCountInString(headerLine[:strings.Index(headerLine, "NAME")])
		for i := 1; i <= len(items); i++ {
			row := stripANSI(lines[i])
			name := items[i-1].Cells[0]
			// Compare display columns (runes): the cursor glyph is multi-byte
			// while occupying a single cell.
			got := utf8.RuneCountInString(row[:strings.Index(row, name)])
			if got != wantColumn {
				t.Errorf("cursor %d: %q starts at column %d, want %d", cursor, row, got, wantColumn)
			}
		}
	}
}

func TestBuildLinesUsesCyanCharactersWithoutBackground(t *testing.T) {
	columns := []string{"NAME"}
	items := []Item{{Cells: []string{"KINGSTON_1"}}, {Cells: []string{"DISCO"}}}
	widths := columnWidths(columns, items)

	lines := buildLines("", columns, items, widths, 0)

	if !strings.Contains(lines[1], cyan) {
		t.Error("the selected row must be coloured cyan")
	}
	if strings.Contains(lines[2], cyan) {
		t.Error("unselected rows must stay in the default (white) colour")
	}
	for _, line := range lines {
		if strings.Contains(line, "\x1b[7m") {
			t.Error("no line may use reverse video (background colouring)")
		}
	}
}

func TestBuildLinesShowsTitleWithoutPrompt(t *testing.T) {
	items := []Item{{Cells: []string{"Mount"}, Desc: "Mount a disk"}}
	widths := columnWidths(nil, items)

	lines := buildLines("GODISK ACTIONS", nil, items, widths, 0)

	// The title lines up with the label text, not with the circle.
	if want := "  " + bold + "GODISK ACTIONS" + reset; lines[0] != want {
		t.Errorf("first line = %q, want %q", lines[0], want)
	}
	row := stripANSI(lines[1])
	labelStart := utf8.RuneCountInString(row[:strings.Index(row, "Mount")])
	if labelStart != 2 {
		t.Errorf("label starts at column %d, want 2", labelStart)
	}
	if titleStart := strings.Index(stripANSI(lines[0]), "GODISK"); titleStart != 2 {
		t.Errorf("title starts at column %d, want 2", titleStart)
	}
	for _, line := range lines {
		if strings.Contains(stripANSI(line), "?") {
			t.Errorf("no prompt line is allowed, got %q", stripANSI(line))
		}
	}
}

func TestPadCellsKeepsColumnsAligned(t *testing.T) {
	widths := []int{10, 4, 5}

	short := padCells([]string{"USB", "usb", "1G"}, widths)
	long := padCells([]string{"KINGSTON_1", "sata", "28.7G"}, widths)

	if len(short) != len(long) {
		t.Errorf("padded rows have different lengths: %q vs %q", short, long)
	}
}

func TestPadRightCountsRunesNotBytes(t *testing.T) {
	got := padRight("ñandú", 8)
	if want := "ñandú   "; got != want {
		t.Errorf("padRight() = %q, want %q", got, want)
	}
}

func TestRenderLineClearsBeforeWritingText(t *testing.T) {
	got := renderLine("● Mount")

	if !strings.HasPrefix(got, clearLine) {
		t.Fatalf("renderLine() = %q, want it to start with %q", got, clearLine)
	}
	if strings.Contains(got[len(clearLine):], clearLine) {
		t.Fatalf("renderLine() = %q, want no erase sequence after the text", got)
	}
	if !strings.HasSuffix(got, "● Mount\r\n") {
		t.Fatalf("renderLine() = %q, want the text followed by CRLF", got)
	}
}

func TestStepWrapsAroundTheEnds(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		count  int
		delta  int
		want   int
	}{
		{name: "up from the first goes to the last", cursor: 0, count: 4, delta: -1, want: 3},
		{name: "down from the last goes to the first", cursor: 3, count: 4, delta: 1, want: 0},
		{name: "down in the middle moves one", cursor: 1, count: 4, delta: 1, want: 2},
		{name: "up in the middle moves one", cursor: 2, count: 4, delta: -1, want: 1},
		{name: "empty list stays at zero", cursor: 0, count: 0, delta: 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := step(tt.cursor, tt.count, tt.delta); got != tt.want {
				t.Errorf("step(%d, %d, %d) = %d, want %d", tt.cursor, tt.count, tt.delta, got, tt.want)
			}
		})
	}
}

func TestStatusLinesUseColourWithoutGlyphs(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "success", got: successLine("mounted"), want: green + "mounted" + reset + "\r\n"},
		{name: "failure", got: failLine("broken"), want: red + "broken" + reset + "\r\n"},
		{name: "warning", got: warnLine("careful"), want: yellow + "careful" + reset + "\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("status line = %q, want %q", tt.got, tt.want)
			}
			if strings.ContainsFunc(tt.got, func(r rune) bool { return r > 127 }) {
				t.Errorf("status line = %q, want no non-ASCII glyphs", tt.got)
			}
		})
	}
}

func TestDebugDelayAcceptsSecondsAndDurations(t *testing.T) {
	tests := []struct {
		raw  string
		want time.Duration
	}{
		{raw: "", want: DefaultDelay},
		{raw: "0", want: 0},
		{raw: "10", want: 10 * time.Second},
		{raw: "500ms", want: 500 * time.Millisecond},
		{raw: "nonsense", want: DefaultDelay},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Setenv("GODISK_DEBUG_DELAY", tt.raw)
			if got := debugDelay(); got != tt.want {
				t.Errorf("debugDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDotsFrameGrowsAndShrinks(t *testing.T) {
	want := []string{".", "..", "...", "..", ".", "", "."}

	for i, expected := range want {
		if got := dotsFrame(i); got != expected {
			t.Errorf("dotsFrame(%d) = %q, want %q", i, got, expected)
		}
	}
}

func TestClearMessagesErasesOnlyThePreviousMessage(t *testing.T) {
	ClearMessages() // start from a clean count

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w

	Success("KINGSTON no errors found")
	ClearMessages()
	Fail("first line\nsecond line")
	ClearMessages()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(out)

	if strings.Contains(text, "\x1b[2J") {
		t.Error("the whole screen must never be cleared")
	}
	// One erase for the single-line success message, two for the two-line fail.
	if got := strings.Count(text, "\x1b[A\r\x1b[2K"); got != 3 {
		t.Errorf("erased %d lines, want 3: %q", got, text)
	}
	// The messages themselves are still written, just preceded by erases.
	for _, want := range []string{"KINGSTON no errors found", "first line\nsecond line"} {
		if !strings.Contains(text, want) {
			t.Errorf("output %q is missing %q", text, want)
		}
	}
}
