package tui

import (
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"

	"github.com/dlvhdr/gh-enhance/internal/api"
	"github.com/dlvhdr/gh-enhance/internal/data"
	"github.com/dlvhdr/gh-enhance/internal/parser"
	"github.com/dlvhdr/gh-enhance/internal/utils"
)

var refreshInterval = time.Second * 15

// --- Message types (some retained for dormant PR-mode Update cases) ---

type workflowRunsFetchedMsg struct {
	pr        api.PRWithChecks
	runs      []data.WorkflowRun
	rateLimit api.RateLimit
	err       error
}

type prChecksIntervalTickMsg struct{ msg tea.Msg }

type startIntervalFetching struct{}

type prFetchedMsg struct {
	pr  api.PR
	err error
}

type runModeFetchedMsg struct {
	runs []data.WorkflowRun
	err  error
}

type runModeIntervalTickMsg struct{ msg tea.Msg }

type jobLogsFetchedMsg struct {
	jobId  string
	logs   []data.LogsWithTime
	steps  []*stepItem
	err    error
	stderr string
}

type checkRunOutputFetchedMsg struct {
	jobId        string
	renderedText string
	text         string
	description  string
	title        string
}

type workflowRunStepsFetchedMsg struct {
	runId string
	data  api.WorkflowRunStepsQuery
}

type checkStepsFetchedMsg struct {
	checkId string
	steps   []api.Step
}

type reRunJobMsg struct {
	jobId string
	err   error
}

type reRunRunMsg struct {
	runId string
	err   error
}

// --- Init & fetching ---

func (m *model) startSpinners() []tea.Cmd {
	return []tea.Cmd{
		m.checksList.StartSpinner(),
		m.runsList.StartSpinner(),
		m.logsSpinner.Tick,
		m.jobsList.StartSpinner(),
	}
}

// makeRunModeInitCmd starts spinners and kicks off the pipeline fetch loop.
// The periodic refresh is self-sustaining: each fetched result re-arms the next
// tick from the Update handler (see scheduleRunRefetch), so init only needs to
// trigger the first fetch.
func (m *model) makeRunModeInitCmd() tea.Cmd {
	cmds := m.startSpinners()
	cmds = append(cmds, m.makeFetchRunCmd())
	return tea.Batch(cmds...)
}

func (m *model) makeFetchRunCmd() tea.Cmd {
	return func() tea.Msg {
		return m.fetchRun()
	}
}

// maxPipelineHistory bounds how many of an MR's pipelines are loaded (newest
// first), to keep the number of per-pipeline job requests reasonable.
const maxPipelineHistory = 20

// fetchRun resolves the target(s) and loads their jobs. In MR mode it returns
// every pipeline of the merge request (newest first); otherwise a single
// pipeline (by id, or the latest for a ref).
func (m *model) fetchRun() tea.Msg {
	if m.pipelineID == 0 && m.mrIID > 0 {
		return m.fetchMRPipelines()
	}

	pid := m.pipelineID
	if pid == 0 {
		p, err := api.GetLatestPipeline(m.repo, m.ref)
		if err != nil {
			return runModeFetchedMsg{err: err}
		}
		pid = p.ID
	}

	pipeline, err := api.GetPipeline(m.repo, pid)
	if err != nil {
		return runModeFetchedMsg{err: err}
	}
	jobs, err := api.ListPipelineJobs(m.repo, pid)
	if err != nil {
		return runModeFetchedMsg{err: err}
	}

	run := buildPipelineRun(pipeline, jobs)
	run.PRNumber = m.mrIID
	return runModeFetchedMsg{runs: []data.WorkflowRun{run}}
}

// fetchMRPipelines loads every pipeline attached to the merge request (capped
// and sorted newest first) along with each pipeline's jobs.
func (m *model) fetchMRPipelines() tea.Msg {
	pipelines, err := api.ListMRPipelines(m.repo, m.mrIID)
	if err != nil {
		return runModeFetchedMsg{err: err}
	}
	if len(pipelines) == 0 {
		return runModeFetchedMsg{err: errors.New("merge request has no pipeline yet")}
	}

	// newest first, then cap to bound the number of job requests
	sort.Slice(pipelines, func(i, j int) bool { return pipelines[i].ID > pipelines[j].ID })
	if len(pipelines) > maxPipelineHistory {
		pipelines = pipelines[:maxPipelineHistory]
	}

	runs := make([]data.WorkflowRun, 0, len(pipelines))
	for _, p := range pipelines {
		jobs, err := api.ListPipelineJobs(m.repo, p.ID)
		if err != nil {
			// Don't fail the whole history because one pipeline's jobs errored.
			log.Error("error listing jobs for pipeline", "pipeline", p.ID, "err", err)
			jobs = nil
		}
		run := buildPipelineRun(p, jobs)
		run.PRNumber = m.mrIID
		runs = append(runs, run)
	}
	data.SortRuns(runs)
	return runModeFetchedMsg{runs: runs}
}

func buildPipelineRun(p api.Pipeline, jobs []api.Job) data.WorkflowRun {
	wfJobs := make([]data.WorkflowJob, 0, len(jobs))
	for _, j := range jobs {
		wfJobs = append(wfJobs, data.WorkflowJob{
			Id:           strconv.FormatInt(j.ID, 10),
			State:        api.StatusFromGitLab(j.Status),
			Conclusion:   api.ConclusionFromGitLab(j.Status),
			Name:         j.Name,
			Stage:        j.Stage,
			Workflow:     p.Ref,
			Event:        p.Source,
			Logs:         []data.LogsWithTime{},
			Link:         j.WebURL,
			Steps:        []api.Step{},
			StartedAt:    j.StartedAt,
			CompletedAt:  j.FinishedAt,
			Bucket:       data.BucketFromStatus(j.Status),
			Kind:         data.JobKindGithubActions,
			AllowFailure: j.AllowFailure,
			IsManual:     j.Status == string(api.StatusGLManual),
			RunNumber:    int(p.IID),
		})
	}
	data.SortJobs(wfJobs)

	return data.WorkflowRun{
		Id:           strconv.FormatInt(p.ID, 10),
		Name:         fmt.Sprintf("#%d %s", p.IID, p.Ref),
		DisplayTitle: p.Ref,
		Link:         p.WebURL,
		Workflow:     p.Source,
		Event:        p.Source,
		Jobs:         wfJobs,
		Bucket:       data.BucketFromStatus(p.Status),
		StartedAt:    p.StartedAt,
		RunNumber:    int(p.IID),
	}
}

// scheduleRunRefetch arms a single delayed refetch of the pipeline. The Update
// handler calls this after every fetched result while the pipeline is still in
// progress, forming a self-sustaining loop that stops on its own once the
// pipeline concludes. The guard lives in the handler (on the up-to-date model),
// not in this closure, so it never fires against a stale snapshot.
func (m *model) scheduleRunRefetch() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return runModeIntervalTickMsg{msg: m.fetchRun()}
	})
}

// fetchRunWithInterval refetches immediately and unconditionally arms one more
// refetch. It is used right after a mutation (retry/play/cancel): the server may
// not have transitioned the job yet, so we cannot trust the current in-progress
// state — we always schedule at least one follow-up, and the handler takes over
// the loop from there.
func (m *model) fetchRunWithInterval() tea.Cmd {
	return tea.Batch(
		m.makeFetchRunCmd(),
		m.scheduleRunRefetch(),
	)
}

// --- Logs / job trace ---

func (m *model) makeFetchJobLogsCmd() tea.Cmd {
	ji := m.getSelectedJobItem()
	if ji == nil {
		return nil
	}
	if ji.isStatusInProgress() || ji.job.IsManual {
		return nil
	}

	log.Info("fetching job trace", "job", ji.job.Name)
	ji.loadingLogs = true
	ji.initiatedLogsFetch = true
	jobID := ji.job.Id
	jobLink := ji.job.Link
	styles := m.styles
	repo := m.repo

	return func() tea.Msg {
		defer utils.TimeTrack(time.Now(), "fetching job trace")
		id, err := strconv.ParseInt(jobID, 10, 64)
		if err != nil {
			return jobLogsFetchedMsg{jobId: jobID, err: err, stderr: err.Error()}
		}
		trace, err := api.GetJobTrace(repo, id)
		if err != nil {
			log.Error("error fetching job trace", "link", jobLink, "err", err)
			return jobLogsFetchedMsg{jobId: jobID, err: err, stderr: err.Error()}
		}

		logs, sections := parser.ParseJobTrace(trace)
		steps := make([]*stepItem, 0, len(sections))
		for _, s := range sections {
			si := NewStepItem(s, jobLink, styles)
			steps = append(steps, &si)
		}
		return jobLogsFetchedMsg{jobId: jobID, logs: logs, steps: steps}
	}
}

// makeFetchWorkflowRunStepsCmd clears the per-run/per-job "loading steps" state.
// Sections are parsed from the job trace, so there is no separate steps request;
// this simply signals the enrichment path to stop showing spinners.
func (m *model) makeFetchWorkflowRunStepsCmd(runId string) tea.Cmd {
	return func() tea.Msg {
		return workflowRunStepsFetchedMsg{runId: runId}
	}
}

// makeFetchCheckStepsCmd is a no-op retained for the dormant flat view.
func (m *model) makeFetchCheckStepsCmd(jobId string) tea.Cmd {
	return nil
}

// makeGetNextPagePRChecksCmd is a no-op retained for the dormant PR view.
func (m *model) makeGetNextPagePRChecksCmd(endCursor string) tea.Cmd {
	return nil
}

// fetchPRChecksWithInterval is a no-op retained for the dormant PR view.
func (m *model) fetchPRChecksWithInterval() tea.Cmd {
	return nil
}

// --- Mutations ---

func (m *model) rerunJob(runId string, jobId string) []tea.Cmd {
	log.Info("retrying job", "runId", runId, "jobId", jobId)
	cmds := make([]tea.Cmd, 0)
	ji := m.getJobItemById(jobId)
	if ji == nil {
		return cmds
	}

	ji.job.Bucket = data.CheckBucketPending
	ji.job.State = api.StatusPending
	ji.job.StartedAt = time.Now()
	ji.job.CompletedAt = time.Time{}
	ji.steps = make([]*stepItem, 0)
	ji.initiatedLogsFetch = false
	ji.renderedLogs = nil
	m.stepsList.ResetSelected()

	id, err := strconv.ParseInt(jobId, 10, 64)
	if err != nil {
		return cmds
	}
	cmds = append(cmds, ji.Tick(), m.inProgressSpinner.Tick, func() tea.Msg {
		return reRunJobMsg{jobId: jobId, err: api.RetryJob(m.repo, id)}
	})
	return cmds
}

func (m *model) rerunRun(runId string) []tea.Cmd {
	log.Info("retrying pipeline", "runId", runId)
	cmds := make([]tea.Cmd, 0)
	ri := m.getRunItemById(runId)
	if ri == nil {
		return cmds
	}

	ri.run.Bucket = data.CheckBucketPending

	id, err := strconv.ParseInt(runId, 10, 64)
	if err != nil {
		return cmds
	}
	cmds = append(cmds, ri.Tick(), func() tea.Msg {
		return reRunRunMsg{runId: runId, err: api.RetryPipeline(m.repo, id)}
	})
	return cmds
}

// playJob triggers a manual job.
func (m *model) playJob(jobId string) []tea.Cmd {
	log.Info("playing manual job", "jobId", jobId)
	cmds := make([]tea.Cmd, 0)
	ji := m.getJobItemById(jobId)
	if ji == nil {
		return cmds
	}

	ji.job.Bucket = data.CheckBucketPending
	ji.job.State = api.StatusPending
	ji.job.IsManual = false
	ji.job.StartedAt = time.Now()

	id, err := strconv.ParseInt(jobId, 10, 64)
	if err != nil {
		return cmds
	}
	cmds = append(cmds, ji.Tick(), m.inProgressSpinner.Tick, func() tea.Msg {
		return reRunJobMsg{jobId: jobId, err: api.PlayJob(m.repo, id)}
	})
	return cmds
}

// cancelJob cancels a running job.
func (m *model) cancelJob(jobId string) []tea.Cmd {
	log.Info("cancelling job", "jobId", jobId)
	cmds := make([]tea.Cmd, 0)
	id, err := strconv.ParseInt(jobId, 10, 64)
	if err != nil {
		return cmds
	}
	cmds = append(cmds, func() tea.Msg {
		return reRunJobMsg{jobId: jobId, err: api.CancelJob(m.repo, id)}
	})
	return cmds
}

// cancelRun cancels a whole pipeline.
func (m *model) cancelRun(runId string) []tea.Cmd {
	log.Info("cancelling pipeline", "runId", runId)
	cmds := make([]tea.Cmd, 0)
	id, err := strconv.ParseInt(runId, 10, 64)
	if err != nil {
		return cmds
	}
	cmds = append(cmds, func() tea.Msg {
		return reRunRunMsg{runId: runId, err: api.CancelPipeline(m.repo, id)}
	})
	return cmds
}

// --- Misc ---

func makeOpenUrlCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return nil
		}
		log.Info("opening url", "url", url)
		openURL(url)
		return nil
	}
}

func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Error("failed opening url", "url", url, "err", err)
	}
}

// mergeWorkflowRuns is retained for the dormant PR view.
func (m *model) mergeWorkflowRuns(msg workflowRunsFetchedMsg) {
	runsMap := make(map[int]data.WorkflowRun)
	for _, run := range m.workflowRuns {
		runsMap[run.RunNumber] = run
	}
	for _, run := range msg.runs {
		runsMap[run.RunNumber] = run
	}
	runs := make([]data.WorkflowRun, 0)
	for _, run := range runsMap {
		run.SortJobs()
		runs = append(runs, run)
	}
	data.SortRuns(runs)
	m.workflowRuns = runs
}

func (m *model) nextPane() pane {
	showSteps := m.shouldShowSteps()
	switch m.focusedPane {
	case PaneRuns:
		return PaneJobs
	case PaneJobs:
		if showSteps {
			return PaneSteps
		}
	case PaneSteps:
		return PaneLogs
	case PaneChecks:
		if showSteps {
			return PaneSteps
		}
		return PaneLogs
	case PaneLogs:
		return PaneLogs
	}
	return PaneLogs
}

func (m *model) previousPane() pane {
	showSteps := m.shouldShowSteps()
	switch m.focusedPane {
	case PaneRuns:
		return PaneRuns
	case PaneJobs:
		return PaneRuns
	case PaneSteps:
		if m.flat {
			return PaneChecks
		}
		return PaneJobs
	case PaneChecks:
		return PaneChecks
	case PaneLogs:
		if showSteps {
			return PaneSteps
		}
		return PaneJobs
	}
	if m.flat {
		return PaneChecks
	}
	return PaneRuns
}
