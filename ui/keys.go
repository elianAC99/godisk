package ui

import (
	"bufio"
	"io"
)

// ReadKey reads a single key press and translates it into a command. Arrow
// keys are reported as "up"/"down", Enter as "enter", Esc as "esc" and q or
// Ctrl-C as "quit".
func ReadKey(r *bufio.Reader) string {
	b, err := r.ReadByte()
	if err != nil {
		return "quit"
	}

	switch b {
	case '\n', '\r':
		return "enter"
	case 3: // Ctrl-C
		return "quit"
	case 'q', 'Q':
		return "quit"
	case 27: // Esc or the start of a CSI sequence
		if r.Buffered() >= 2 {
			seq := make([]byte, 2)
			if _, err := io.ReadFull(r, seq); err == nil {
				switch seq[0] {
				case '[', 'O':
					switch seq[1] {
					case 'A':
						return "up"
					case 'B':
						return "down"
					}
				}
			}
		}
		return "esc"
	}
	return ""
}
