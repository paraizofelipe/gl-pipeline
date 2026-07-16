package api

import (
	"io"
	"strings"
	"time"

	"charm.land/log/v2"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	glclient "github.com/dlvhdr/gh-enhance/internal/gitlab"
)

// --- Normalized status/conclusion vocabulary consumed by the TUI ---
//
// GitLab reports a single status string per job/pipeline. To keep the TUI
// (which was built around GitHub's split of "status" + "conclusion") working
// unchanged, we normalize each GitLab status into a Status (lifecycle) and a
// Conclusion (terminal result).

type Status string

type Conclusion string

const (
	StatusQueued     Status = "QUEUED"
	StatusCompleted  Status = "COMPLETED"
	StatusInProgress Status = "IN_PROGRESS"
	StatusRequested  Status = "REQUESTED"
	StatusWaiting    Status = "WAITING"
	StatusPending    Status = "PENDING"

	ConclusionActionRequired Conclusion = "ACTION_REQUIRED"
	ConclusionCancelled      Conclusion = "CANCELLED"
	ConclusionFailure        Conclusion = "FAILURE"
	ConclusionNeutral        Conclusion = "NEUTRAL"
	ConclusionSkipped        Conclusion = "SKIPPED"
	ConclusionStale          Conclusion = "STALE"
	ConclusionStartupFailure Conclusion = "STARTUP_FAILURE"
	ConclusionSuccess        Conclusion = "SUCCESS"
	ConclusionTimedOut       Conclusion = "TIMED_OUT"
)

// PipelineStatus is the raw GitLab status vocabulary (lowercase strings).
type PipelineStatus string

const (
	StatusGLCreated            PipelineStatus = "created"
	StatusGLWaitingForResource PipelineStatus = "waiting_for_resource"
	StatusGLPreparing          PipelineStatus = "preparing"
	StatusGLPending            PipelineStatus = "pending"
	StatusGLRunning            PipelineStatus = "running"
	StatusGLSuccess            PipelineStatus = "success"
	StatusGLFailed             PipelineStatus = "failed"
	StatusGLCanceled           PipelineStatus = "canceled"
	StatusGLCanceling          PipelineStatus = "canceling"
	StatusGLSkipped            PipelineStatus = "skipped"
	StatusGLManual             PipelineStatus = "manual"
	StatusGLScheduled          PipelineStatus = "scheduled"
	StatusGLWaitingForCallback PipelineStatus = "waiting_for_callback"
)

func IsFailureConclusion(c Conclusion) bool {
	switch c {
	case ConclusionActionRequired, ConclusionFailure,
		ConclusionStartupFailure, ConclusionTimedOut:
		return true
	default:
		return false
	}
}

// StatusFromGitLab maps a GitLab status onto the lifecycle Status the TUI uses
// to decide whether to show a spinner.
func StatusFromGitLab(status string) Status {
	switch PipelineStatus(status) {
	case StatusGLRunning, StatusGLCanceling:
		return StatusInProgress
	case StatusGLPending, StatusGLCreated, StatusGLPreparing,
		StatusGLWaitingForResource, StatusGLWaitingForCallback:
		return StatusPending
	case StatusGLScheduled:
		return StatusWaiting
	default: // success, failed, canceled, skipped, manual
		return StatusCompleted
	}
}

// ConclusionFromGitLab maps a GitLab status onto a terminal Conclusion. Returns
// an empty Conclusion for non-terminal states.
func ConclusionFromGitLab(status string) Conclusion {
	switch PipelineStatus(status) {
	case StatusGLSuccess:
		return ConclusionSuccess
	case StatusGLFailed:
		return ConclusionFailure
	case StatusGLCanceled, StatusGLCanceling:
		return ConclusionCancelled
	case StatusGLSkipped:
		return ConclusionSkipped
	case StatusGLManual:
		return ConclusionNeutral
	default:
		return ""
	}
}

// Step is a section within a job trace (the GitLab equivalent of a build step).
type Step struct {
	Conclusion  Conclusion
	Name        string
	Number      int
	StartedAt   time.Time
	CompletedAt time.Time
	Status      Status
}

// --- Domain values passed from the api layer to the tui layer ---

type Pipeline struct {
	ID        int64
	IID       int64
	Ref       string
	SHA       string
	Status    string
	Source    string
	WebURL    string
	CreatedAt time.Time
	StartedAt time.Time
	MRIID     int
}

type Job struct {
	ID           int64
	Name         string
	Stage        string
	Status       string
	WebURL       string
	AllowFailure bool
	StartedAt    time.Time
	FinishedAt   time.Time
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// GetPipeline fetches a single pipeline by numeric ID.
func GetPipeline(project string, pipelineID int64) (Pipeline, error) {
	c, err := glclient.RESTClient()
	if err != nil {
		return Pipeline{}, err
	}
	p, _, err := c.Pipelines.GetPipeline(project, pipelineID)
	if err != nil {
		log.Error("error fetching pipeline", "err", err)
		return Pipeline{}, err
	}
	return Pipeline{
		ID:        p.ID,
		IID:       p.IID,
		Ref:       p.Ref,
		SHA:       p.SHA,
		Status:    strings.ToLower(p.Status),
		Source:    string(p.Source),
		WebURL:    p.WebURL,
		CreatedAt: deref(p.CreatedAt),
		StartedAt: deref(p.StartedAt),
	}, nil
}

// GetLatestPipeline returns the latest pipeline for a ref (branch/tag).
func GetLatestPipeline(project string, ref string) (Pipeline, error) {
	c, err := glclient.RESTClient()
	if err != nil {
		return Pipeline{}, err
	}
	opt := &gitlab.GetLatestPipelineOptions{}
	if ref != "" {
		opt.Ref = gitlab.Ptr(ref)
	}
	p, _, err := c.Pipelines.GetLatestPipeline(project, opt)
	if err != nil {
		log.Error("error fetching latest pipeline", "err", err)
		return Pipeline{}, err
	}
	return Pipeline{
		ID:        p.ID,
		IID:       p.IID,
		Ref:       p.Ref,
		SHA:       p.SHA,
		Status:    strings.ToLower(p.Status),
		Source:    string(p.Source),
		WebURL:    p.WebURL,
		CreatedAt: deref(p.CreatedAt),
		StartedAt: deref(p.StartedAt),
	}, nil
}

// ListMRPipelines returns every pipeline attached to a merge request, newest
// first. Used to populate the pipeline-history pane in MR mode.
func ListMRPipelines(project string, mrIID int) ([]Pipeline, error) {
	c, err := glclient.RESTClient()
	if err != nil {
		return nil, err
	}
	infos, _, err := c.MergeRequests.ListMergeRequestPipelines(project, int64(mrIID))
	if err != nil {
		log.Error("error listing MR pipelines", "err", err)
		return nil, err
	}
	pipelines := make([]Pipeline, 0, len(infos))
	for _, p := range infos {
		pipelines = append(pipelines, Pipeline{
			ID:        p.ID,
			IID:       p.IID,
			Ref:       p.Ref,
			SHA:       p.SHA,
			Status:    strings.ToLower(p.Status),
			Source:    p.Source,
			WebURL:    p.WebURL,
			CreatedAt: deref(p.CreatedAt),
			// PipelineInfo carries no started_at; use created_at so the run
			// list has a sensible timestamp to display.
			StartedAt: deref(p.CreatedAt),
			MRIID:     mrIID,
		})
	}
	return pipelines, nil
}

// ListPipelineJobs lists all jobs of a pipeline, following pagination so that
// jobs on later pages are never silently dropped.
func ListPipelineJobs(project string, pipelineID int64) ([]Job, error) {
	c, err := glclient.RESTClient()
	if err != nil {
		return nil, err
	}
	opts := &gitlab.ListJobsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}
	result := make([]Job, 0)
	for {
		jobs, resp, err := c.Jobs.ListPipelineJobs(project, pipelineID, opts)
		if err != nil {
			log.Error("error listing pipeline jobs", "err", err)
			return nil, err
		}
		for _, j := range jobs {
			result = append(result, Job{
				ID:           j.ID,
				Name:         j.Name,
				Stage:        j.Stage,
				Status:       strings.ToLower(j.Status),
				WebURL:       j.WebURL,
				AllowFailure: j.AllowFailure,
				StartedAt:    deref(j.StartedAt),
				FinishedAt:   deref(j.FinishedAt),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

// GetJobTrace fetches the raw trace (log) of a job.
func GetJobTrace(project string, jobID int64) (string, error) {
	c, err := glclient.RESTClient()
	if err != nil {
		return "", err
	}
	reader, _, err := c.Jobs.GetTraceFile(project, jobID)
	if err != nil {
		log.Error("error fetching job trace", "err", err)
		return "", err
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// --- Mutations ---

func RetryJob(project string, jobID int64) error {
	c, err := glclient.RESTClient()
	if err != nil {
		return err
	}
	_, _, err = c.Jobs.RetryJob(project, jobID)
	return err
}

func PlayJob(project string, jobID int64) error {
	c, err := glclient.RESTClient()
	if err != nil {
		return err
	}
	_, _, err = c.Jobs.PlayJob(project, jobID, nil)
	return err
}

func CancelJob(project string, jobID int64) error {
	c, err := glclient.RESTClient()
	if err != nil {
		return err
	}
	_, _, err = c.Jobs.CancelJob(project, jobID)
	return err
}

func RetryPipeline(project string, pipelineID int64) error {
	c, err := glclient.RESTClient()
	if err != nil {
		return err
	}
	_, _, err = c.Pipelines.RetryPipelineBuild(project, pipelineID)
	return err
}

func CancelPipeline(project string, pipelineID int64) error {
	c, err := glclient.RESTClient()
	if err != nil {
		return err
	}
	_, _, err = c.Pipelines.CancelPipelineBuild(project, pipelineID)
	return err
}
