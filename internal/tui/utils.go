package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-enhance/internal/api"
	"github.com/dlvhdr/gh-enhance/internal/data"
)

// accStats aggregates job/check counts by outcome for the (dormant) footer
// summary. Retained so the inherited PR-checks footer keeps compiling.
type accStats struct {
	Failed     int
	InProgress int
	Succeeded  int
	Skipped    int
}

func accumulatedStats(a, b []api.ContextCountByState) accStats {
	var s accStats
	all := make([]api.ContextCountByState, 0, len(a)+len(b))
	all = append(all, a...)
	all = append(all, b...)
	for _, c := range all {
		switch c.State {
		case "FAILURE", "ERROR", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			s.Failed += c.Count
		case "IN_PROGRESS", "PENDING", "QUEUED", "WAITING":
			s.InProgress += c.Count
		case "SUCCESS":
			s.Succeeded += c.Count
		case "SKIPPED", "NEUTRAL":
			s.Skipped += c.Count
		}
	}
	return s
}

func bucketToIcon(bucket data.CheckBucket, initialStyle lipgloss.Style, styles styles) string {
	switch bucket {
	case data.CheckBucketPass:
		return styles.successGlyph.Inherit(initialStyle).Render()
	case data.CheckBucketFail:
		return styles.failureGlyph.Inherit(initialStyle).Render()
	case data.CheckBucketNeutral:
		return styles.neutralGlyph.Inherit(initialStyle).Render()
	case data.CheckBucketSkipping:
		return styles.skippedGlyph.Inherit(initialStyle).Render()
	case data.CheckBucketCancel:
		return styles.canceledGlyph.Inherit(initialStyle).Render()
	default:
		return styles.pendingGlyph.Inherit(initialStyle).Render()
	}
}
