package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	glclient "github.com/dlvhdr/gh-enhance/internal/gitlab"
)

// newMockServer wires a fake GitLab REST API and injects a client pointed at it,
// exercising the full fetch path (client-go REST -> our api layer) end to end.
func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/pipelines/123/jobs"):
			w.Write([]byte(`[
				{"id":1,"name":"build","stage":"build","status":"success","web_url":"https://gl/j/1","allow_failure":false,"started_at":"2024-01-01T10:00:00Z","finished_at":"2024-01-01T10:01:00Z"},
				{"id":2,"name":"deploy","stage":"deploy","status":"manual","web_url":"https://gl/j/2","allow_failure":true}
			]`))
		case strings.HasSuffix(path, "/merge_requests/5/pipelines"):
			// intentionally returned oldest-first to exercise sorting
			w.Write([]byte(`[
				{"id":100,"iid":1,"ref":"feature","status":"success","source":"merge_request_event","web_url":"https://gl/p/100"},
				{"id":123,"iid":3,"ref":"feature","status":"running","source":"merge_request_event","web_url":"https://gl/p/123"},
				{"id":110,"iid":2,"ref":"feature","status":"failed","source":"merge_request_event","web_url":"https://gl/p/110"}
			]`))
		case strings.HasSuffix(path, "/pipelines/123"):
			w.Write([]byte(`{"id":123,"iid":7,"ref":"main","sha":"abc","status":"running","source":"push","web_url":"https://gl/p/123","created_at":"2024-01-01T09:59:00Z","started_at":"2024-01-01T10:00:00Z"}`))
		case strings.HasSuffix(path, "/jobs/1/trace"):
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("section_start:1700000000:build\r\x1b[0KBuilding\nmake\nsection_end:1700000060:build\r\x1b[0K\n"))
		default:
			http.Error(w, "not found: "+path, http.StatusNotFound)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := gitlab.NewClient("test-token", gitlab.WithBaseURL(srv.URL+"/api/v4"))
	if err != nil {
		t.Fatalf("failed building client: %v", err)
	}
	glclient.SetClient(c)
	t.Cleanup(func() { glclient.SetClient(nil) })
	return srv
}

func TestFetchPipelineEndToEnd(t *testing.T) {
	newMockServer(t)
	const project = "group/project"

	pipeline, err := GetPipeline(project, 123)
	if err != nil {
		t.Fatalf("GetPipeline: %v", err)
	}
	if pipeline.IID != 7 || pipeline.Status != "running" || pipeline.Ref != "main" {
		t.Fatalf("unexpected pipeline: %+v", pipeline)
	}

	jobs, err := ListPipelineJobs(project, 123)
	if err != nil {
		t.Fatalf("ListPipelineJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Stage != "build" || jobs[0].Status != "success" {
		t.Errorf("unexpected first job: %+v", jobs[0])
	}
	if jobs[1].Status != "manual" || !jobs[1].AllowFailure {
		t.Errorf("unexpected manual job: %+v", jobs[1])
	}
}

func TestListMRPipelinesEndToEnd(t *testing.T) {
	newMockServer(t)

	pipelines, err := ListMRPipelines("group/project", 5)
	if err != nil {
		t.Fatalf("ListMRPipelines: %v", err)
	}
	if len(pipelines) != 3 {
		t.Fatalf("expected 3 pipelines, got %d", len(pipelines))
	}
	for _, p := range pipelines {
		if p.MRIID != 5 {
			t.Errorf("expected MRIID 5, got %d", p.MRIID)
		}
	}
	// caller (fetchMRPipelines) sorts newest-first by ID; verify the ids are all present
	ids := map[int64]bool{}
	for _, p := range pipelines {
		ids[p.ID] = true
	}
	for _, want := range []int64{100, 110, 123} {
		if !ids[want] {
			t.Errorf("missing pipeline id %d", want)
		}
	}
}

func TestGetJobTraceEndToEnd(t *testing.T) {
	newMockServer(t)

	trace, err := GetJobTrace("group/project", 1)
	if err != nil {
		t.Fatalf("GetJobTrace: %v", err)
	}
	if !strings.Contains(trace, "Building") || !strings.Contains(trace, "make") {
		t.Errorf("unexpected trace content: %q", trace)
	}
}
