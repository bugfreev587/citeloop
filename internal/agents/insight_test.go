package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/crawl"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestSummarizeCrawlCapturesOperatorRelevantFields(t *testing.T) {
	summary := summarizeCrawl("https://example.com", &crawl.Result{
		Landing:    &crawl.Page{URL: "https://example.com", Title: "Home"},
		Discovered: []string{"https://example.com/blog/a", "https://example.com/blog/b"},
		Articles: []*crawl.Page{
			{URL: "https://example.com/blog/a", Title: "A"},
			{URL: "https://example.com/blog/b", Title: "B"},
		},
		Truncated: true,
		Errors:    []string{"skip https://example.com/blog/c: timeout"},
	})
	summary.InventoryCount = 2

	if summary.LandingURL != "https://example.com" {
		t.Fatalf("landing url = %q", summary.LandingURL)
	}
	if summary.DiscoveredCount != 2 || summary.FetchedCount != 2 || summary.InventoryCount != 2 {
		t.Fatalf("counts = discovered %d fetched %d inventory %d", summary.DiscoveredCount, summary.FetchedCount, summary.InventoryCount)
	}
	if !summary.Truncated {
		t.Fatal("summary must expose truncated crawls")
	}
	if len(summary.Errors) != 1 {
		t.Fatalf("errors len = %d, want 1", len(summary.Errors))
	}
	if len(summary.SampleURLs) != 2 || summary.SampleURLs[0] != "https://example.com/blog/a" {
		t.Fatalf("sample urls = %#v", summary.SampleURLs)
	}
}

func TestProfileSourceURLsLandingOnlyForQuickProfile(t *testing.T) {
	urls := profileSourceURLs("https://example.com/input", &crawl.Result{
		Landing: &crawl.Page{URL: "https://example.com"},
		Articles: []*crawl.Page{
			{URL: "https://example.com/blog/a"},
			{URL: "https://example.com/blog/b"},
		},
	}, false)

	if len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("quick profile source urls = %#v, want landing only", urls)
	}
}

func TestProfileSourceURLsIncludesArticlesForFullProfile(t *testing.T) {
	urls := profileSourceURLs("https://example.com/input", &crawl.Result{
		Landing: &crawl.Page{URL: "https://example.com"},
		Articles: []*crawl.Page{
			nil,
			{URL: "https://example.com/blog/a"},
			{URL: ""},
			{URL: "https://example.com/blog/b"},
		},
	}, true)

	want := []string{"https://example.com", "https://example.com/blog/a", "https://example.com/blog/b"}
	if len(urls) != len(want) {
		t.Fatalf("source urls = %#v, want %#v", urls, want)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Fatalf("source urls = %#v, want %#v", urls, want)
		}
	}
}

func TestRunInventoryFromCrawlRecordsCrawlFailureRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := &insightDBSpy{}
	insight := NewInsight(Deps{Q: db.New(store), LLM: &sequenceLLM{}}, nil)
	projectID := uuid.New()

	_, _, err := insight.RunInventoryFromCrawl(context.Background(), projectID, srv.URL, config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		MaxPages:         1,
		SitemapURLCap:    1,
	})
	if err == nil {
		t.Fatal("RunInventoryFromCrawl must return crawl errors")
	}

	run := store.findRun("crawl")
	if run == nil {
		t.Fatalf("expected crawl failure generation run, got %#v", store.runs)
	}
	if run.status != "error" {
		t.Fatalf("crawl run status = %q, want error", run.status)
	}
	if run.err == nil || !strings.Contains(*run.err, "crawl:") {
		t.Fatalf("crawl run error = %#v, want crawl error", run.err)
	}
}

func TestRunInventoryFromCrawlUpgradesProfileWithArticleSources(t *testing.T) {
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body><main>Landing positioning.</main></body></html>`))
		case "/sitemap.xml":
			w.Header().Set("content-type", "application/xml")
			_, _ = w.Write([]byte(`<urlset><url><loc>` + baseURL + `/blog/a</loc></url></urlset>`))
		case "/blog/a":
			_, _ = w.Write([]byte(`<html><head><title>Article A</title></head><body><main>Article-specific proof for the full profile.</main></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	store := &insightDBSpy{}
	provider := &sequenceLLM{resps: []string{
		`{"title":"Article A","target_keyword":"proof","topics":["proof"],"summary":"Article summary","evidence_snippets":["Article-specific proof for the full profile."]}`,
		`{"positioning":"Full profile","value_props":["Proof"],"features":["Inventory"],"icp":["Operators"],"tone":"clear","key_terms":["proof"],"competitors":[],"differentiators":["Full corpus"]}`,
	}}
	insight := NewInsight(Deps{Q: db.New(store), LLM: provider}, nil)

	_, _, err := insight.RunInventoryFromCrawl(context.Background(), uuid.New(), srv.URL, config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		MaxPages:         2,
		MaxDepth:         1,
		SitemapURLCap:    10,
	})
	if err != nil {
		t.Fatalf("RunInventoryFromCrawl: %v", err)
	}

	if len(store.profileInserts) != 1 {
		t.Fatalf("profile inserts = %d, want 1", len(store.profileInserts))
	}
	if !strings.Contains(string(store.profileInserts[0].sourceURLs), "/blog/a") {
		t.Fatalf("full profile source urls = %s, want article URL", string(store.profileInserts[0].sourceURLs))
	}
	if run := store.findRun("profile"); run == nil || !strings.Contains(string(run.input), "full_crawl") {
		t.Fatalf("expected full_crawl profile run, got %#v", store.runs)
	}
	if len(provider.reqs) < 2 || !strings.Contains(provider.reqs[1].Prompt, "Article-specific proof") {
		t.Fatalf("full profile prompt did not include article corpus: %#v", provider.reqs)
	}
}

type capturedRun struct {
	input  []byte
	status string
	err    *string
}

type capturedProfileInsert struct {
	sourceURLs json.RawMessage
}

type insightDBSpy struct {
	runs           []capturedRun
	profileInserts []capturedProfileInsert
}

func (s *insightDBSpy) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *insightDBSpy) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (s *insightDBSpy) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "insert into generation_runs"):
		status, _ := args[7].(string)
		errValue, _ := args[8].(*string)
		input, _ := args[2].([]byte)
		s.runs = append(s.runs, capturedRun{input: input, status: status, err: errValue})
		return scanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(string), args[2].([]byte), args[3].([]byte),
			args[4].(*string), args[5].(*int32), args[6].(pgtype.Numeric), status, errValue, pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "select coalesce(max(version)"):
		return scanRow{values: []any{int32(2)}}
	case strings.Contains(query, "insert into product_profiles"):
		sourceURLs, _ := args[1].(json.RawMessage)
		s.profileInserts = append(s.profileInserts, capturedProfileInsert{sourceURLs: sourceURLs})
		return scanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(json.RawMessage), args[2].(json.RawMessage),
			args[3].(int32), true, pgtype.Timestamptz{}, pgtype.Timestamptz{},
		}}
	case strings.Contains(query, "insert into content_inventory"):
		return scanRow{values: []any{
			uuid.New(), args[0].(uuid.UUID), args[1].(string), args[2].(*string), args[3].(*string),
			args[4].(json.RawMessage), args[5].(*string), args[6].(json.RawMessage), args[7].(string),
			pgtype.Timestamptz{},
		}}
	default:
		return scanRow{err: pgx.ErrNoRows}
	}
}

func (s *insightDBSpy) findRun(step string) *capturedRun {
	needle := `"step":"` + step + `"`
	for i := range s.runs {
		if strings.Contains(string(s.runs[i].input), needle) {
			return &s.runs[i]
		}
	}
	return nil
}

type scanRow struct {
	values []any
	err    error
}

func (r scanRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *uuid.UUID:
			*d = r.values[i].(uuid.UUID)
		case *[]byte:
			*d = r.values[i].([]byte)
		case **string:
			*d = r.values[i].(*string)
		case **int32:
			*d = r.values[i].(*int32)
		case *pgtype.Numeric:
			*d = r.values[i].(pgtype.Numeric)
		case *string:
			*d = r.values[i].(string)
		case *pgtype.Timestamptz:
			*d = r.values[i].(pgtype.Timestamptz)
		case *int32:
			*d = r.values[i].(int32)
		case *bool:
			*d = r.values[i].(bool)
		default:
			return nil
		}
	}
	return nil
}
