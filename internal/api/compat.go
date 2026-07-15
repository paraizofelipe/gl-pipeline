package api

import "time"

// This file holds dependency-free shapes that the TUI layer still references on
// code paths inherited from the GitHub origin (the PR-checks and flat views).
// gl-pipeline always runs in pipeline mode, so these paths are dormant; the
// types are kept only so the TUI keeps compiling without pulling in any GitHub
// client dependency. They are slated for removal once the TUI is fully ported.

type CommitState string

const (
	CommitStateExpected CommitState = "EXPECTED"
	CommitStateError    CommitState = "ERROR"
	CommitStateFailure  CommitState = "FAILURE"
	CommitStatePending  CommitState = "PENDING"
	CommitStateSuccess  CommitState = "SUCCESS"
)

// ContextCountByState mirrors the shape of the old check-count aggregation.
type ContextCountByState struct {
	State string
	Count int
}

type PageInfo struct {
	EndCursor       string
	HasNextPage     bool
	HasPreviousPage bool
}

type CheckRun struct {
	Id          string
	Name        string
	Status      Status
	Title       string
	Url         string
	DetailsUrl  string
	Conclusion  Conclusion
	DatabaseId  int
	StartedAt   time.Time
	CompletedAt time.Time
}

type ContextNode struct {
	Typename      string
	CheckRun      CheckRun
	StatusContext struct {
		Context     string
		Description string
		State       Conclusion
	}
}

type CheckRunWithSteps struct {
	Id         string
	DatabaseId int
	Url        string
	Steps      struct {
		Nodes []Step
	}
}

type statusCheckRollup struct {
	State    CommitState
	Contexts struct {
		TotalCount                 int
		CheckRunCount              int
		CheckRunCountsByState      []ContextCountByState
		StatusContextCount         int
		StatusContextCountsByState []ContextCountByState
		Nodes                      []ContextNode
		PageInfo                   PageInfo
	}
}

type commitNode struct {
	Commit struct {
		StatusCheckRollup statusCheckRollup
	}
}

type PR struct {
	Title      string
	Number     int
	Url        string
	Repository struct {
		NameWithOwner string
	}
	Merged      bool
	IsDraft     bool
	Closed      bool
	HeadRefName string
	Commits     struct {
		Nodes []commitNode
	}
}

type PRWithChecks struct {
	Title      string
	Number     int
	Url        string
	Repository struct {
		NameWithOwner string
	}
	Merged      bool
	IsDraft     bool
	Closed      bool
	HeadRefName string
	Commits     struct {
		Nodes []commitNode
	}
}

func (pr *PRWithChecks) IsStatusCheckInProgress() bool {
	return false
}

type RateLimit struct {
	Remaining int64
	ResetAt   time.Time
}

// WorkflowRunStepsQuery is the dormant shape consumed by the (unused) steps
// enrichment path.
type WorkflowRunStepsQuery struct {
	Resource struct {
		WorkflowRun struct {
			CheckSuite struct {
				CheckRuns struct {
					Nodes []CheckRunWithSteps
				}
			}
		}
	}
}
