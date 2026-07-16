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
//	section_start:<unix_ts>:<section_name>[collapsed=true]\r\x1b[0K<header text>
//	section_end:<unix_ts>:<section_name>\r\x1b[0K
//
// section names use only [A-Za-z0-9_.-] plus an optional [key=value] suffix. The
// visible header follows the `\r\x1b[0K` (carriage-return + erase-to-end-of-line)
// sequence.
var sectionRe = regexp.MustCompile(
	`section_(start|end):(\d+):([A-Za-z0-9_.\-]+)(?:\[[^\]]*\])?\r?(?:\x1b\[0?K)?`,
)

// markerNormalizeRe matches a section marker that is NOT already at the start of
// a physical line — the transition case where GitLab packs the previous
// section's `section_end` and the next `section_start` onto one line separated
// only by `\r` (e.g. `…section_end:…\r\x1b[0Ksection_start:…\r\x1b[0Kheader`).
// Requiring the trailing `\r` anchors the match to a genuine marker so log
// content that merely mentions the words is never split. The captured leading
// byte is preserved; a newline is inserted before the marker so the line-based
// parser sees exactly one marker per line.
var markerNormalizeRe = regexp.MustCompile(
	`([^\n])(section_(?:start|end):[0-9]+:[A-Za-z0-9_.\-]+(?:\[[^\]]*\])?\r)`,
)

// ansiAllRe matches every ANSI CSI sequence, including SGR colors. It is used to
// derive a plain-text section header for the step name (colors belong in the
// log body, not in the list label).
var ansiAllRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

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

	// GitLab often emits a section's `section_end` and the next `section_start`
	// on the same physical line (separated by `\r`). Put each marker on its own
	// line first, so the line-based scan below sees them all instead of only the
	// first one per line.
	trace = markerNormalizeRe.ReplaceAllString(trace, "$1\n$2")

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
				// the list label is plain text; the colored header stays in the log
				label := strings.TrimSpace(ansiAllRe.ReplaceAllString(header, ""))
				sections = append(sections, api.Step{
					Name:      headerOrName(label, name),
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
