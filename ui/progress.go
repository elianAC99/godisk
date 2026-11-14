package ui

import "time"

// progressFrames grow the dots one by one and then erase them one by one:
// ".", "..", "...", "..", ".", "".
var progressFrames = []string{".", "..", "...", "..", ".", ""}

// Progress shows the running action with a dot animation as work runs, then
// erases the line so only the result remains.
func Progress(msg string, work func() (string, error)) (string, error) {
	type result struct {
		detail string
		err    error
	}

	done := make(chan result, 1)
	go func() {
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
