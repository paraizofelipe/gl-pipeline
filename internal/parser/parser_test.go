package parser

import (
	"strings"
	"testing"

	"github.com/dlvhdr/gh-enhance/internal/data"
)

func TestParseJobTraceSections(t *testing.T) {
	// Two sections plus a plain line. Uses the real GitLab marker format:
	// section_start:<ts>:<name>\r\x1b[0K<header>
	trace := "section_start:1700000000:prepare\r\x1b[0KPreparing environment\n" +
		"Running with gitlab-runner\n" +
		"section_end:1700000005:prepare\r\x1b[0K\n" +
		"section_start:1700000005:build\r\x1b[0KBuilding\n" +
		"make build\n" +
		"section_end:1700000030:build\r\x1b[0K\n"

	logs, sections := ParseJobTrace(trace)

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Name != "Preparing environment" {
		t.Errorf("expected first section header, got %q", sections[0].Name)
	}
	if sections[1].Name != "Building" {
		t.Errorf("expected second section header, got %q", sections[1].Name)
	}
	if sections[0].CompletedAt.IsZero() {
		t.Errorf("expected first section to be closed")
	}
	if sections[1].StartedAt.Unix() != 1700000005 {
		t.Errorf("expected build section start ts, got %d", sections[1].StartedAt.Unix())
	}

	// group markers should be present in the parsed log stream
	var groupStarts int
	for _, l := range logs {
		if l.Kind == data.LogKindGroupStart {
			groupStarts++
		}
	}
	if groupStarts != 2 {
		t.Errorf("expected 2 group-start log lines, got %d", groupStarts)
	}
}

func TestParseJobTraceCollapsesProgressBars(t *testing.T) {
	// A pip-style progress bar redraws one physical line with \r frames.
	trace := "0.0/5.5 MB ? eta -:--:--\r2.1/5.5 MB 6.8 MB/s\r5.5/5.5 MB 9.6 MB/s 0:00:01\n"
	logs, _ := ParseJobTrace(trace)
	if len(logs) == 0 {
		t.Fatal("expected a log line")
	}
	got := logs[0].Log
	if strings.ContainsRune(got, '\r') {
		t.Errorf("carriage return leaked into output: %q", got)
	}
	if got != "5.5/5.5 MB 9.6 MB/s 0:00:01" {
		t.Errorf("expected only the final progress frame, got %q", got)
	}
}

func TestParseJobTraceKeepsColorsStripsCursor(t *testing.T) {
	// green "ok" wrapped in SGR, preceded by a cursor-column control sequence.
	trace := "\x1b[1G\x1b[32mok\x1b[0m done\n"
	logs, _ := ParseJobTrace(trace)
	if len(logs) == 0 {
		t.Fatal("expected a log line")
	}
	got := logs[0].Log
	if strings.Contains(got, "\x1b[1G") {
		t.Errorf("cursor-move sequence not stripped: %q", got)
	}
	if !strings.Contains(got, "\x1b[32m") || !strings.Contains(got, "\x1b[0m") {
		t.Errorf("SGR color sequences should be preserved, got %q", got)
	}
}

func TestParseJobTraceStripsEraseSequences(t *testing.T) {
	trace := "hello\x1b[0K world\n"
	logs, _ := ParseJobTrace(trace)
	if len(logs) == 0 {
		t.Fatal("expected at least one log line")
	}
	if logs[0].Log != "hello world" {
		t.Errorf("expected erase sequence stripped, got %q", logs[0].Log)
	}
}
