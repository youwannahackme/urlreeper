package output

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"
)

// ANSI color escape codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
)

var ansiEscapeRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Result represents a single found URL and its metadata
type Result struct {
	Source     string `json:"source,omitempty"`
	Method     string `json:"method,omitempty"`
	URL        string `json:"url"`
	Body       string `json:"body,omitempty"`
	Depth      int    `json:"depth,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

// Writer handles concurrent writing to stdout and/or a file
type Writer struct {
	mu         sync.Mutex
	file       *os.File
	jsonOutput bool
	verbose    bool
	noColor    bool
}

// NewWriter creates a new output writer
func NewWriter(outputPath string, jsonOutput, verbose, noColor bool) (*Writer, error) {
	var f *os.File
	var err error
	if outputPath != "" {
		f, err = os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open output file: %w", err)
		}
	}

	return &Writer{
		file:       f,
		jsonOutput: jsonOutput,
		verbose:    verbose,
		noColor:    noColor,
	}, nil
}

// Close closes the file handle if it exists
func (w *Writer) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Write outputs a result to stdout and the output file
func (w *Writer) Write(res *Result) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var stdoutLine string
	var fileLine string

	if w.jsonOutput {
		data, err := json.Marshal(res)
		if err != nil {
			return
		}
		stdoutLine = string(data)
		fileLine = stdoutLine
	} else {
		// Terminal output with colors
		var sourcePart, methodPart, depthPart, statusPart string
		if res.Source != "" {
			if w.noColor {
				sourcePart = fmt.Sprintf("[%s] ", res.Source)
			} else {
				sourcePart = fmt.Sprintf("%s[%s]%s ", colorBlue, res.Source, colorReset)
			}
		}
		if res.Method != "" {
			if w.noColor {
				methodPart = fmt.Sprintf("[%s] ", res.Method)
			} else {
				methodPart = fmt.Sprintf("%s[%s]%s ", colorGreen, res.Method, colorReset)
			}
		}
		if res.StatusCode != 0 {
			if w.noColor {
				statusPart = fmt.Sprintf("[%d] ", res.StatusCode)
			} else {
				var color string
				switch {
				case res.StatusCode >= 200 && res.StatusCode < 300:
					color = colorGreen
				case res.StatusCode >= 300 && res.StatusCode < 400:
					color = colorYellow
				default:
					color = colorRed
				}
				statusPart = fmt.Sprintf("%s[%d]%s ", color, res.StatusCode, colorReset)
			}
		}
		if w.verbose && res.Depth > 0 {
			if w.noColor {
				depthPart = fmt.Sprintf(" [depth:%d]", res.Depth)
			} else {
				depthPart = fmt.Sprintf(" %s[depth:%d]%s", colorYellow, res.Depth, colorReset)
			}
		}

		stdoutLine = fmt.Sprintf("%s%s%s%s%s", sourcePart, methodPart, statusPart, res.URL, depthPart)

		// File output (always strip colors and keep it clean)
		if w.file != nil {
			fileLine = res.URL
		}
	}

	// Write to stdout
	fmt.Println(stdoutLine)

	// Write to file if configured
	if w.file != nil {
		_, _ = w.file.WriteString(fileLine + "\n")
	}
}
