package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestTopicMutationRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "update topic",
			method: http.MethodPut,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic",
		},
		{
			name:   "schedule topic",
			method: http.MethodPost,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic/schedule",
		},
		{
			name:   "archive topic",
			method: http.MethodPost,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic/archive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, want %d", tt.name, res.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGenerateTopicRouteStartsBackgroundGeneration(t *testing.T) {
	source, err := os.ReadFile("handlers_agents.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)

	if !strings.Contains(body, "http.StatusAccepted") {
		t.Fatal("generate topic should accept background generation without holding the HTTP request open")
	}
	if !strings.Contains(body, "startTopicGeneration") {
		t.Fatal("generate topic should dispatch writer and QA work outside the request path")
	}
	if strings.Contains(body, "arts, err := ag.Generate(r.Context(), id, topic)") {
		t.Fatal("generate topic must not run Writer+QA synchronously on the request context")
	}
}

func TestGenerateTopicReconcilesExistingDraftTopic(t *testing.T) {
	projectID := uuid.New()
	topicID := uuid.New()
	topic := db.Topic{
		ID:            topicID,
		ProjectID:     projectID,
		Channel:       "blog",
		Title:         "Already drafted",
		InternalLinks: json.RawMessage("[]"),
		Status:        "backlog",
	}
	article := db.Article{
		ID:        uuid.New(),
		ProjectID: projectID,
		TopicID:   topicID,
		Kind:      "canonical",
		ContentMd: "draft",
		SeoMeta:   json.RawMessage("{}"),
		QaIssues:  json.RawMessage("[]"),
		Status:    "pending_review",
	}
	fakeDB := &generateTopicDB{topic: topic, articles: []db.Article{article}}
	srv := &Server{Q: db.New(fakeDB)}

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/topics/"+topicID.String()+"/generate", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("projectID", projectID.String())
	routeCtx.URLParams.Add("topicID", topicID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	res := httptest.NewRecorder()

	srv.generateTopic(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Status   string       `json:"status"`
		Topic    db.Topic     `json:"topic"`
		Articles []db.Article `json:"articles"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ready" {
		t.Fatalf("response status = %q, want ready", body.Status)
	}
	if body.Topic.Status != "drafted" {
		t.Fatalf("topic status = %q, want drafted", body.Topic.Status)
	}
	if len(body.Articles) != 1 {
		t.Fatalf("articles = %d, want 1", len(body.Articles))
	}
	if got := fakeDB.statusUpdates; len(got) != 1 || got[0] != "drafted" {
		t.Fatalf("status updates = %#v, want [drafted]", got)
	}
}

type generateTopicDB struct {
	topic         db.Topic
	articles      []db.Article
	statusUpdates []string
}

func (f *generateTopicDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *generateTopicDB) Query(_ context.Context, sql string, _ ...interface{}) (pgx.Rows, error) {
	if strings.Contains(sql, "ListArticlesByTopicForProject") {
		rows := make([][]any, 0, len(f.articles))
		for _, article := range f.articles {
			rows = append(rows, articleScanValues(article))
		}
		return &stubRows{rows: rows}, nil
	}
	return &stubRows{}, nil
}

func (f *generateTopicDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(sql, "GetTopicForProject"):
		return stubRow{values: topicScanValues(f.topic)}
	case strings.Contains(sql, "UpdateTopicStatusForProject"):
		status := args[2].(string)
		f.statusUpdates = append(f.statusUpdates, status)
		updated := f.topic
		updated.Status = status
		f.topic = updated
		return stubRow{values: topicScanValues(updated)}
	default:
		return stubRow{err: pgx.ErrNoRows}
	}
}

type stubRow struct {
	values []any
	err    error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	scanValues(dest, r.values)
	return nil
}

type stubRows struct {
	rows   [][]any
	cursor int
	closed bool
}

func (r *stubRows) Close()                                       { r.closed = true }
func (r *stubRows) Err() error                                   { return nil }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Next() bool {
	if r.cursor >= len(r.rows) {
		r.closed = true
		return false
	}
	r.cursor++
	return true
}
func (r *stubRows) Scan(dest ...any) error {
	scanValues(dest, r.rows[r.cursor-1])
	return nil
}
func (r *stubRows) Values() ([]any, error) { return r.rows[r.cursor-1], nil }
func (r *stubRows) RawValues() [][]byte    { return nil }
func (r *stubRows) Conn() *pgx.Conn        { return nil }

func scanValues(dest []any, values []any) {
	for i, value := range values {
		target := reflect.ValueOf(dest[i]).Elem()
		if value == nil {
			target.Set(reflect.Zero(target.Type()))
			continue
		}
		target.Set(reflect.ValueOf(value))
	}
}

func topicScanValues(topic db.Topic) []any {
	return []any{
		topic.ID,
		topic.ProjectID,
		topic.Channel,
		topic.Title,
		topic.TargetKeyword,
		topic.TargetPrompt,
		topic.Angle,
		topic.Format,
		topic.Priority,
		topic.InternalLinks,
		topic.Status,
		topic.ScheduledAt,
		topic.CreatedAt,
	}
}

func articleScanValues(article db.Article) []any {
	return []any{
		article.ID,
		article.ProjectID,
		article.TopicID,
		article.Kind,
		article.Platform,
		article.ContentMd,
		article.SeoMeta,
		article.GeoScore,
		article.SeoScore,
		article.QaIssues,
		article.QaBlocking,
		article.CanonicalUrl,
		article.Status,
		article.ScheduledAt,
		article.ReviewedBy,
		article.ReviewedAt,
		article.PublishedAt,
		article.PublishResult,
		article.LastPublishError,
		article.PublishAttempts,
		article.NextPublishRetryAt,
		article.PublishPhase,
		article.ResolvedSlug,
		article.PublishPath,
		article.CanonicalUrlVerifiedAt,
		article.LastPublishRunID,
		article.CreatedAt,
		article.ContentHash,
		article.RepairAttempts,
		article.LastRepairAt,
		article.RepairStatus,
		article.RepairFailureReason,
		article.RequiresHumanDecision,
		article.HumanDecisionOptions,
		article.QaFeedback,
	}
}
