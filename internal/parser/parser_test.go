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

func TestParseJobTracePacksMarkersOnOneLine(t *testing.T) {
	// Real GitLab traces pack a section's section_end and the next section_start
	// onto a single physical line, separated only by \r. Colors also leak into
	// the header. The parser must still surface every section with a plain name.
	esc := "\x1b"
	trace := "section_start:1700000000:prepare_executor\r" + esc + "[0K" + esc +
		"[36;1mPreparing the \"kubernetes\" executor" + esc + "[0;m\n" +
		"Using namespace foo\n" +
		"section_end:1700000010:prepare_executor\r" + esc + "[0Ksection_start:1700000010:get_sources\r" +
		esc + "[0K" + esc + "[36;1mGetting source from Git repository" + esc + "[0;m\n" +
		"Fetching changes...\n" +
		"section_end:1700000020:get_sources\r" + esc + "[0Ksection_start:1700000020:step_script\r" +
		esc + "[0K" + esc + "[36;1mExecuting \"step_script\" stage" + esc + "[0;m\n" +
		"make build\n" +
		"section_end:1700000050:step_script\r" + esc + "[0K\n"

	_, sections := ParseJobTrace(trace)

	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}
	wantNames := []string{
		"Preparing the \"kubernetes\" executor",
		"Getting source from Git repository",
		`Executing "step_script" stage`,
	}
	for i, want := range wantNames {
		if sections[i].Name != want {
			t.Errorf("section[%d].Name = %q, want %q", i, sections[i].Name, want)
		}
		if strings.Contains(sections[i].Name, "\x1b") {
			t.Errorf("section[%d].Name leaked ANSI: %q", i, sections[i].Name)
		}
		if sections[i].CompletedAt.IsZero() {
			t.Errorf("section[%d] (%s) was not closed", i, sections[i].Name)
		}
	}
	if sections[2].StartedAt.Unix() != 1700000020 || sections[2].CompletedAt.Unix() != 1700000050 {
		t.Errorf("step_script timing wrong: start=%d end=%d",
			sections[2].StartedAt.Unix(), sections[2].CompletedAt.Unix())
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
