package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
)

func TestAdminSiteFixMigrationRoutesAreAdminRegistered(t *testing.T) {
	projectID, batchID := uuid.New(), uuid.New()
	router := (&Server{SiteFixMigration: fakeAdminMigrationService{}}).Router()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/admin/projects/" + projectID.String() + "/site-fix-migration/dry-run"},
		{http.MethodPost, "/api/admin/projects/" + projectID.String() + "/site-fix-migration/apply"},
		{http.MethodPost, "/api/admin/projects/" + projectID.String() + "/site-fix-migration/" + batchID.String() + "/rollback"},
		{http.MethodGet, "/api/admin/projects/" + projectID.String() + "/site-fix-migration/" + batchID.String()},
		{http.MethodGet, "/api/admin/projects/" + projectID.String() + "/site-fix-migration/reviews"},
		{http.MethodPost, "/api/admin/projects/" + projectID.String() + "/site-fix-migration/reviews/" + batchID.String() + "/resolve"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{"expected_snapshot_hash":"snapshot"}`))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s not registered: %d", tc.method, tc.path, res.Code)
		}
	}
	server, _ := os.ReadFile("server.go")
	text := string(server)
	if !strings.Contains(text, `r.Use(s.requireAdmin)`) || !strings.Contains(text, `/site-fix-migration/dry-run`) {
		t.Fatal("migration routes must remain inside requireAdmin group")
	}
}

func TestAdminMigrationReviewOperationsExposeAgeSLAAndAudit(t *testing.T) {
	handler, err := os.ReadFile("admin_site_fix_migration.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(handler)
	for _, required := range []string{
		"listAdminMigrationReviews",
		"resolveAdminMigrationReview",
		"min_age_seconds",
		"overdue_only",
		"age_seconds",
		"sla_status",
		"internal_owner",
		"adminAuditActor",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration review operations missing %q", required)
		}
	}
}

func TestAdminSiteFixMigrationMutationsRequireExpectedSnapshotAndMapDriftTo409(t *testing.T) {
	projectID, batchID := uuid.New(), uuid.New()
	service := fakeAdminMigrationService{applyErr: sitefix.ErrMigrationSnapshotDrift, rollbackErr: sitefix.ErrMigrationSnapshotDrift}
	router := (&Server{SiteFixMigration: service}).Router()
	for _, path := range []string{
		"/api/admin/projects/" + projectID.String() + "/site-fix-migration/apply",
		"/api/admin/projects/" + projectID.String() + "/site-fix-migration/" + batchID.String() + "/rollback",
	} {
		for _, body := range []string{`{}`, `{"expected_snapshot_hash":"stale"}`} {
			res := httptest.NewRecorder()
			router.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
			if res.Code != http.StatusConflict {
				t.Fatalf("POST %s body=%s got %d want 409: %s", path, body, res.Code, res.Body.String())
			}
		}
	}
}

func TestAdminSiteFixMigrationErrorsAreRedacted(t *testing.T) {
	projectID := uuid.New()
	router := (&Server{SiteFixMigration: fakeAdminMigrationService{dryErr: errors.New("postgres password=secret")}}).Router()
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/admin/projects/"+projectID.String()+"/site-fix-migration/dry-run", nil))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", res.Code)
	}
	if strings.Contains(res.Body.String(), "password") || strings.Contains(res.Body.String(), "secret") {
		t.Fatalf("internal error leaked: %s", res.Body.String())
	}
}

type fakeAdminMigrationService struct{ dryErr, applyErr, rollbackErr, reportErr error }

func (f fakeAdminMigrationService) DryRun(context.Context, uuid.UUID, string) (sitefix.MigrationDryRunReport, error) {
	return sitefix.MigrationDryRunReport{SnapshotHash: "snapshot"}, f.dryErr
}
func (f fakeAdminMigrationService) Apply(context.Context, uuid.UUID, string, string) (sitefix.MigrationBatchReport, error) {
	return sitefix.MigrationBatchReport{}, f.applyErr
}
func (f fakeAdminMigrationService) Rollback(context.Context, uuid.UUID, uuid.UUID, string, string) (sitefix.MigrationRollbackReport, error) {
	return sitefix.MigrationRollbackReport{}, f.rollbackErr
}
func (f fakeAdminMigrationService) Report(context.Context, uuid.UUID, uuid.UUID) (sitefix.MigrationBatchReport, error) {
	return sitefix.MigrationBatchReport{}, f.reportErr
}
