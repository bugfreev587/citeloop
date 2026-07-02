package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/google/uuid"
)

type fakeSEORunner struct {
	calls         []string
	errAt         string
	doctorCreated bool
}

func (f *fakeSEORunner) Sync(ctx context.Context, projectID uuid.UUID, siteURL string) (seopkg.SyncResult, error) {
	f.calls = append(f.calls, "sync:"+siteURL)
	if f.errAt == "sync" {
		return seopkg.SyncResult{}, errors.New("sync failed")
	}
	return seopkg.SyncResult{Status: "ok"}, nil
}

func (f *fakeSEORunner) Analyze(ctx context.Context, projectID uuid.UUID) (seopkg.SyncResult, error) {
	f.calls = append(f.calls, "analyze")
	if f.errAt == "analyze" {
		return seopkg.SyncResult{}, errors.New("analyze failed")
	}
	return seopkg.SyncResult{Status: "ok"}, nil
}

func (f *fakeSEORunner) Brief(ctx context.Context, projectID uuid.UUID) (seopkg.Brief, error) {
	f.calls = append(f.calls, "brief")
	if f.errAt == "brief" {
		return seopkg.Brief{}, errors.New("brief failed")
	}
	return seopkg.Brief{Mode: "opportunities"}, nil
}

func (f *fakeSEORunner) StartDoctorRun(ctx context.Context, req seopkg.DoctorRunRequest) (db.SeoDoctorRun, bool, error) {
	f.calls = append(f.calls, "doctor_start:"+string(req.Trigger))
	if f.errAt == "doctor_start" {
		return db.SeoDoctorRun{}, false, errors.New("doctor start failed")
	}
	return db.SeoDoctorRun{ID: uuid.New(), ProjectID: req.ProjectID, Trigger: string(req.Trigger)}, f.doctorCreated, nil
}

func (f *fakeSEORunner) RunDoctor(ctx context.Context, projectID, runID uuid.UUID) (seopkg.DoctorReport, error) {
	f.calls = append(f.calls, "doctor_run")
	if f.errAt == "doctor_run" {
		return seopkg.DoctorReport{}, errors.New("doctor run failed")
	}
	return seopkg.DoctorReport{}, nil
}

func TestRunSEOForProjectRunsSyncAnalyzeBrief(t *testing.T) {
	runner := &fakeSEORunner{}
	s := &Scheduler{
		BlogBaseURL: "https://unipost.dev",
		seoRunnerFactory: func(q *db.Queries) seoRunner {
			return runner
		},
	}

	err := s.runSEOForProject(context.Background(), nil, db.Project{ID: uuid.New()})
	if err != nil {
		t.Fatalf("runSEOForProject returned error: %v", err)
	}
	want := []string{"sync:https://unipost.dev", "analyze", "brief"}
	if len(runner.calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	for i := range want {
		if runner.calls[i] != want[i] {
			t.Fatalf("calls = %#v, want %#v", runner.calls, want)
		}
	}
}

func TestRunSEODoctorForProjectStartsAndRunsWeeklyDoctor(t *testing.T) {
	runner := &fakeSEORunner{doctorCreated: true}
	projectID := uuid.New()
	s := &Scheduler{
		seoRunnerFactory: func(q *db.Queries) seoRunner {
			return runner
		},
	}

	err := s.runSEODoctorForProject(context.Background(), nil, db.Project{ID: projectID})
	if err != nil {
		t.Fatalf("runSEODoctorForProject returned error: %v", err)
	}
	want := []string{"doctor_start:weekly", "doctor_run"}
	if len(runner.calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	for i := range want {
		if runner.calls[i] != want[i] {
			t.Fatalf("calls = %#v, want %#v", runner.calls, want)
		}
	}
}

func TestRunSEODoctorForProjectReturnsActiveRunWithoutDuplicateExecution(t *testing.T) {
	runner := &fakeSEORunner{}
	projectID := uuid.New()
	s := &Scheduler{
		seoRunnerFactory: func(q *db.Queries) seoRunner {
			return runner
		},
	}
	err := s.runSEODoctorForProject(context.Background(), nil, db.Project{ID: projectID})
	if err != nil {
		t.Fatalf("runSEODoctorForProject returned error: %v", err)
	}
	want := []string{"doctor_start:weekly"}
	if len(runner.calls) != len(want) || runner.calls[0] != want[0] {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestRunSEOForProjectStopsOnSyncError(t *testing.T) {
	runner := &fakeSEORunner{errAt: "sync"}
	s := &Scheduler{
		BlogBaseURL: "https://unipost.dev",
		seoRunnerFactory: func(q *db.Queries) seoRunner {
			return runner
		},
	}

	err := s.runSEOForProject(context.Background(), nil, db.Project{ID: uuid.New()})
	if err == nil {
		t.Fatal("runSEOForProject returned nil, want sync error")
	}
	if len(runner.calls) != 1 || runner.calls[0] != "sync:https://unipost.dev" {
		t.Fatalf("calls = %#v, want sync only", runner.calls)
	}
}
