// Package readline provides interactive line editing for vish.
package readline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/peterh/liner"
)

// Reader wraps liner to provide readline functionality with history.
type Reader struct {
	state       *liner.State
	historyFile string
}

// New creates a new Reader with history support.
func New() (*Reader, error) {
	state := liner.NewLiner()
	state.SetCtrlCAborts(true)

	r := &Reader{state: state}

	// Load history
	home, err := os.UserHomeDir()
	if err == nil {
		r.historyFile = filepath.Join(home, ".vish_history")
		if f, err := os.Open(r.historyFile); err == nil {
			state.ReadHistory(f)
			f.Close()
		}
	}

	return r, nil
}

// Readline reads a line with the given prompt.
// Returns (line, nil) on success, ("", io.EOF) on EOF, ("", err) on error.
func (r *Reader) Readline(prompt string) (string, error) {
	line, err := r.state.Prompt(prompt)
	if err != nil {
		if err == liner.ErrPromptAborted {
			fmt.Fprintln(os.Stderr)
			return "", nil // Return empty line on Ctrl-C
		}
		return "", io.EOF
	}
	if line != "" {
		r.state.AppendHistory(line)
	}
	return line, nil
}

// Close saves history and closes the reader.
func (r *Reader) Close() {
	if r.historyFile != "" {
		if f, err := os.Create(r.historyFile); err == nil {
			r.state.WriteHistory(f)
			f.Close()
		}
	}
	r.state.Close()
}
