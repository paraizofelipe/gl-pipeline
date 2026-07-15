package data

import (
	"sort"
	"strings"
	"time"

	"github.com/dlvhdr/gh-enhance/internal/api"
)

// WorkflowRun models a GitLab pipeline. The type name is kept for continuity
// with the TUI, which consumes this shape; semantically it is a pipeline that
// holds the jobs that ran as part of it.
type WorkflowRun struct {
	Id           string
	Name         string
	DisplayTitle string
	Link         string
	Workflow     string
	Event        string // pipeline source (push, merge_request_event, ...)
	Jobs         []WorkflowJob
	Bucket       CheckBucket
	StartedAt    time.Time
	RunNumber    int // pipeline IID
	PRNumber     int // merge request IID, when known
}

// WorkflowJob models a single GitLab CI job. Its Steps are the sections parsed
// from the job trace, and its Logs hold the parsed trace lines.
type WorkflowJob struct {
	Id           string
	State        api.Status
	Conclusion   api.Conclusion
	Name         string
	Title        string
	Stage        string // GitLab stage the job belongs to (build, test, ...)
	Workflow     string
	PendingEnv   string
	Event        string
	Logs         []LogsWithTime
	Link         string
	Steps        []api.Step
	StartedAt    time.Time
	CompletedAt  time.Time
	Bucket       CheckBucket
	Kind         JobKind
	AllowFailure bool
	IsManual     bool

	// RunNumber uniquely identifies the parent pipeline (its IID).
	RunNumber int
}

type LogKind int

const (
	LogKindStepNone LogKind = iota
	LogKindStepStart
	LogKindGroupStart
	LogKindGroupEnd
	LogKindCommand
	LogKindError
	LogKindJobCleanup
	LogKindCompleteJob
)

type LogsWithTime struct {
	Log   string
	Time  time.Time
	Kind  LogKind
	Depth int
}

type JobKind int

const (
	JobKindCheckRun JobKind = iota
	JobKindGithubActions
	JobKindExternal
)

type CheckBucket int

const (
	CheckBucketPass = iota
	CheckBucketSkipping
	CheckBucketFail
	CheckBucketCancel
	CheckBucketPending
	CheckBucketNeutral
)

// BucketFromStatus maps a GitLab pipeline/job status (lowercase) to a bucket.
func BucketFromStatus(status string) CheckBucket {
	switch api.PipelineStatus(status) {
	case api.StatusGLSuccess:
		return CheckBucketPass
	case api.StatusGLFailed:
		return CheckBucketFail
	case api.StatusGLCanceled, api.StatusGLCanceling:
		return CheckBucketCancel
	case api.StatusGLSkipped:
		return CheckBucketSkipping
	case api.StatusGLManual:
		return CheckBucketNeutral
	default:
		// created, waiting_for_resource, preparing, pending, running,
		// scheduled, waiting_for_callback
		return CheckBucketPending
	}
}

func (run WorkflowRun) SortJobs() {
	SortJobs(run.Jobs)
}

// Order: failed -> in progress -> skipped -> neutral -> haven't started -> started at -> name
func SortJobs(jobs []WorkflowJob) {
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Bucket == CheckBucketFail &&
			jobs[j].Bucket != CheckBucketFail {
			return true
		}
		if jobs[j].Bucket == CheckBucketFail &&
			jobs[i].Bucket != CheckBucketFail {
			return false
		}

		if jobs[i].State == api.StatusInProgress &&
			jobs[j].State != api.StatusInProgress {
			return true
		}
		if jobs[j].State == api.StatusInProgress &&
			jobs[i].State != api.StatusInProgress {
			return false
		}

		if jobs[i].Conclusion == api.ConclusionSkipped &&
			jobs[j].Conclusion != api.ConclusionSkipped {
			return true
		}
		if jobs[j].Conclusion == api.ConclusionSkipped &&
			jobs[i].Conclusion != api.ConclusionSkipped {
			return false
		}

		if jobs[i].Conclusion == api.ConclusionNeutral &&
			jobs[j].Conclusion != api.ConclusionNeutral {
			return true
		}
		if jobs[j].Conclusion == api.ConclusionNeutral &&
			jobs[i].Conclusion != api.ConclusionNeutral {
			return false
		}

		if jobs[i].StartedAt.IsZero() {
			return false
		}
		// if second job hasn't started yet, it should appear last
		if jobs[j].StartedAt.IsZero() {
			return true
		}

		if jobs[i].StartedAt.Equal(jobs[j].StartedAt) {
			return strings.Compare(jobs[i].Name, jobs[j].Name) < 0
		}

		return jobs[i].StartedAt.Before(jobs[j].StartedAt)
	})
}

func (job WorkflowJob) IsStatusInProgress() bool {
	return job.State == api.StatusInProgress
}

func SortRuns(runs []WorkflowRun) {
	sort.SliceStable(runs, func(i, j int) bool {
		// newest pipeline first (higher IID)
		return runs[i].RunNumber > runs[j].RunNumber
	})
}
