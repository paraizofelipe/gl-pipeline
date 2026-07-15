package data

import "testing"

func TestBucketFromStatus(t *testing.T) {
	cases := map[string]CheckBucket{
		"success":   CheckBucketPass,
		"failed":    CheckBucketFail,
		"canceled":  CheckBucketCancel,
		"skipped":   CheckBucketSkipping,
		"manual":    CheckBucketNeutral,
		"running":   CheckBucketPending,
		"pending":   CheckBucketPending,
		"created":   CheckBucketPending,
		"scheduled": CheckBucketPending,
	}
	for status, want := range cases {
		if got := BucketFromStatus(status); got != want {
			t.Errorf("BucketFromStatus(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestSortRunsNewestFirst(t *testing.T) {
	runs := []WorkflowRun{
		{RunNumber: 1},
		{RunNumber: 3},
		{RunNumber: 2},
	}
	SortRuns(runs)
	if runs[0].RunNumber != 3 || runs[1].RunNumber != 2 || runs[2].RunNumber != 1 {
		t.Errorf("expected runs sorted 3,2,1 got %d,%d,%d",
			runs[0].RunNumber, runs[1].RunNumber, runs[2].RunNumber)
	}
}
