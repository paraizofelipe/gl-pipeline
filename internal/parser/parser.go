package parser

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dlvhdr/gh-enhance/internal/api"
	"github.com/dlvhdr/gh-enhance/internal/data"
)

// GitLab job traces delimit collapsible sections with control lines of the form
//
//	section_start:<unix_ts>:<section_name>\r\x1b[0K<header text>
//	section_end:<unix_ts>:<section_name>\r\x1b[0K
//
// section names use only [A-Za-z0-9_.-]. The visible header follows the
// `\r\x1b[0K` (carriage-return + erase-to-end-of-line) sequence.
var sectionRe = regexp.MustCompile(
	`section_(start|end):(\d+):([A-Za-z0-9_.\-]+)\r?(?:\x1b\[0?K)?`,
)

// eraseLineRe matches the standalone erase-to-end-of-line sequence that GitLab
// sprinkles through traces; stripping it keeps colors intact while removing
// cursor noise.
var eraseLineRe = regexp.MustCompile(`\x1b\[0?K`)

// ansiControlRe matches ANSI CSI sequences for cursor movement, erasing and
// mode toggles (final bytes A-H, J, K, S, T, f, n, h, l) but deliberately NOT
// SGR color sequences (final byte 'm'), so colors survive while cursor noise
// that would corrupt the layout is removed.
var ansiControlRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-HJKSTfnhl]`)

// Log markers referenced by the TUI log renderer. GitLab traces don't embed
// these literal tokens (sections are stripped during parsing), so the renderer's
// replacements against them are harmless no-ops kept for rendering symmetry.
const (
	GroupStartMarker = "##[group]"
	CommandMarker    = "[command]"
	ErrorMarker      = "##[error]"
)

// ParseJobTrace parses a raw GitLab job trace into renderable log lines and the
// list of trace sections (surfaced in the TUI as the equivalent of steps).
func ParseJobTrace(trace string) ([]data.LogsWithTime, []api.Step) {
	logs := make([]data.LogsWithTime, 0)
	sections := make([]api.Step, 0)

	// index of the open section in `sections`, by section name
	open := make(map[string]int)
	depth := 0
	stepNumber := 0
	var lastTime time.Time

	for _, line := range strings.Split(trace, "\n") {
		line = strings.TrimRight(line, "\r")

		if m := sectionRe.FindStringSubmatch(line); m != nil {
			kind := m[1]
			ts, _ := strconv.ParseInt(m[2], 10, 64)
			name := m[3]
			at := time.Unix(ts, 0)
			lastTime = at

			// visible text after the marker (a header for section_start)
			header := strings.TrimSpace(sectionRe.ReplaceAllString(line, ""))
			header = eraseLineRe.ReplaceAllString(header, "")

			if kind == "start" {
				depth++
				stepNumber++
				sections = append(sections, api.Step{
					Name:      headerOrName(header, name),
					Number:    stepNumber,
					StartedAt: at,
					Status:    api.StatusInProgress,
				})
				open[name] = len(sections) - 1
				logs = append(logs, data.LogsWithTime{
					Log:   headerOrName(header, name),
					Time:  at,
					Kind:  data.LogKindGroupStart,
					Depth: depth,
				})
			} else {
				if idx, ok := open[name]; ok {
					sections[idx].CompletedAt = at
					sections[idx].Status = api.StatusCompleted
					sections[idx].Conclusion = api.ConclusionSuccess
					delete(open, name)
				}
				logs = append(logs, data.LogsWithTime{Time: at, Kind: data.LogKindGroupEnd})
				depth = max(0, depth-1)
			}
			continue
		}

		// Collapse carriage-return overwrites: progress bars redraw the same
		// physical line with \r, so only the content after the last \r is what a
		// terminal would ultimately show. Left in, a \r resets the cursor to
		// screen column 0 and the text bleeds across the other panes.
		if i := strings.LastIndex(line, "\r"); i >= 0 {
			line = line[i+1:]
		}

		text := ansiControlRe.ReplaceAllString(line, "")
		text = eraseLineRe.ReplaceAllString(text, "")
		kind := data.LogKindStepNone
		if isErrorLine(text) {
			kind = data.LogKindError
		}
		logs = append(logs, data.LogsWithTime{
			Log:   strings.TrimRight(text, "\n"),
			Time:  lastTime,
			Kind:  kind,
			Depth: depth,
		})
	}

	return logs, sections
}

func headerOrName(header, name string) string {
	if header != "" {
		return header
	}
	return name
}

func isErrorLine(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "error:") ||
		strings.HasPrefix(strings.TrimSpace(lower), "error ") ||
		strings.Contains(text, "ERROR:") ||
		strings.Contains(lower, "command terminated with")
}
