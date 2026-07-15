package tui

import (
	"testing"

	"github.com/dlvhdr/gh-enhance/internal/api"
	"github.com/dlvhdr/gh-enhance/internal/data"
)

func TestBuildPipelineRun(t *testing.T) {
	p := api.Pipeline{
		ID:     100,
		IID:    7,
		Ref:    "feature/x",
		Status: "running",
		Source: "merge_request_event",
		WebURL: "https://gitlab.com/g/p/-/pipelines/100",
	}
	jobs := []api.Job{
		{ID: 1, Name: "build", Stage: "build", Status: "success"},
		{ID: 2, Name: "deploy", Stage: "deploy", Status: "manual"},
		{ID: 3, Name: "test", Stage: "test", Status: "failed"},
	}

	run := buildPipelineRun(p, jobs)

	if run.Id != "100" {
		t.Fatalf("expected run Id 100, got %s", run.Id)
	}
	if run.RunNumber != 7 {
		t.Fatalf("expected RunNumber 7, got %d", run.RunNumber)
	}
	if run.Bucket != data.CheckBucketPending {
		t.Fatalf("expected pending bucket for running pipeline, got %d", run.Bucket)
	}
	if len(run.Jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(run.Jobs))
	}

	byName := map[string]data.WorkflowJob{}
	for _, j := range run.Jobs {
		byName[j.Name] = j
	}

	if byName["build"].Bucket != data.CheckBucketPass {
		t.Errorf("build should be pass bucket, got %d", byName["build"].Bucket)
	}
	if byName["test"].Bucket != data.CheckBucketFail {
		t.Errorf("test should be fail bucket, got %d", byName["test"].Bucket)
	}
	if !byName["deploy"].IsManual {
		t.Errorf("deploy should be flagged manual")
	}
	if byName["deploy"].Stage != "deploy" {
		t.Errorf("deploy stage should be preserved, got %s", byName["deploy"].Stage)
	}

	// failed jobs sort first
	if run.Jobs[0].Name != "test" {
		t.Errorf("expected failed job to sort first, got %s", run.Jobs[0].Name)
	}
}

func TestMergeWorkflowRuns(t *testing.T) {
	m := NewModel("group/project", "", ModelOpts{})

	first := data.WorkflowRun{Id: "1", Name: "#1 main", RunNumber: 1}
	m.mergeWorkflowRuns(workflowRunsFetchedMsg{runs: []data.WorkflowRun{first}})
	if len(m.workflowRuns) != 1 {
		t.Fatalf("expected 1 run, got %d", len(m.workflowRuns))
	}

	second := data.WorkflowRun{Id: "2", Name: "#2 main", RunNumber: 2}
	m.mergeWorkflowRuns(workflowRunsFetchedMsg{runs: []data.WorkflowRun{second}})
	if len(m.workflowRuns) != 2 {
		t.Fatalf("expected 2 runs after merge, got %d", len(m.workflowRuns))
	}

	// newest pipeline (higher IID) should sort first
	if m.workflowRuns[0].RunNumber != 2 {
		t.Errorf("expected newest run first, got RunNumber %d", m.workflowRuns[0].RunNumber)
	}
}
