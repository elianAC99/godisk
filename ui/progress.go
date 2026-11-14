package ui

import (
	"os"
	"strconv"
	"time"
)

// DefaultDelay is how long every action waits before running, so the loading
// state can be inspected. Override it with GODISK_DEBUG_DELAY ("0" disables
// the wait, "10s", "500ms" and a plain number of seconds are also accepted).
const DefaultDelay = 30 * time.Second

// progressFrames grow the dots one by one and then erase them one by one:
// ".", "..", "...", "..", ".", "".
var progressFrames = []string{".", "..", "...", "..", ".", ""}

// Progress shows the running action with a dot animation, waits the debug
// delay, runs work and erases the line afterwards so only the result remains.
func Progress(msg string, work func() (string, error)) (string, error) {
	type result struct {
		detail string
		err    error
	}

	done := make(chan result, 1)
	go func() {
		time.Sleep(debugDelay())
		detail, err := work()
		done <- result{detail, err}
	}()

	// Keep the block cursor off the dots while the animation runs.
	write(hideCursor)
	defer write(showCursor)

	draw := func(dots string) {
		write("\r\x1b[K" + cyan + msg + dots + reset)
	}

	draw(dotsFrame(0))
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for i := 1; ; i++ {
		select {
		case res := <-done:
			write("\r\x1b[K")
			return res.detail, res.err
		case <-ticker.C:
			draw(dotsFrame(i))
		}
	}
}

// dotsFrame returns the dots of the i-th animation frame.
func dotsFrame(i int) string {
	return progressFrames[i%len(progressFrames)]
}

// debugDelay reads GODISK_DEBUG_DELAY, accepting a Go duration ("10s",
// "500ms") or a plain number of seconds ("10"). It defaults to DefaultDelay.
func debugDelay() time.Duration {
	raw := os.Getenv("GODISK_DEBUG_DELAY")
	if raw == "" {
		return DefaultDelay
	}
	if delay, err := time.ParseDuration(raw); err == nil {
		return delay
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return DefaultDelay
}
