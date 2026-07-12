package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/crawl"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
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

func TestRunQuickProfileRecordsProvisionalStage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body><main>Landing positioning for a provisional profile.</main></body></html>`))
	}))
	defer srv.Close()

	store := &insightDBSpy{}
	provider := &sequenceLLM{resps: []string{
		`{"positioning":"Provisional profile","value_props":["Fast"],"features":["Landing"],"icp":["Operators"],"tone":"clear","key_terms":["profile"],"competitors":[],"differentiators":["Quick context"]}`,
	}}
	insight := NewInsight(Deps{Q: db.New(store), LLM: provider}, nil)

	_, _, err := insight.RunQuickProfile(context.Background(), uuid.New(), srv.URL, config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		RespectRobots:    false,
	})
	if err != nil {
		t.Fatalf("RunQuickProfile: %v", err)
	}

	run := store.findRun("profile")
	if run == nil {
		t.Fatalf("expected profile generation run, got %#v", store.runs)
	}
	if !strings.Contains(string(run.input), `"scope":"landing"`) {
		t.Fatalf("profile run input = %s, want landing scope", string(run.input))
	}
	output := string(run.output)
	for _, want := range []string{`"profile_source":"landing"`, `"profile_stage":"provisional"`, `"duration_ms":`} {
		if !strings.Contains(output, want) {
			t.Fatalf("profile run output = %s, want %s", output, want)
		}
	}
}

func TestRunQuickProfileRecordsStartAndLandingFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := &insightDBSpy{}
	insight := NewInsight(Deps{Q: db.New(store), LLM: &sequenceLLM{}}, nil)
	projectID := uuid.New()

	_, _, err := insight.RunQuickProfile(context.Background(), projectID, srv.URL, config.CrawlConfig{
		RequestTimeoutMs: 1000,
		RateLimitRPS:     1000,
		RespectRobots:    false,
	})
	if err == nil {
		t.Fatal("RunQuickProfile must return landing fetch errors")
	}

	start := store.findRunWith("profile", `"phase":"started"`)
	if start == nil {
		t.Fatalf("expected profile start generation run, got %#v", store.runs)
	}
	failure := store.findRunWith("profile", `"phase":"fetch_landing"`)
	if failure == nil {
		t.Fatalf("expected profile landing failure generation run, got %#v", store.runs)
	}
	if failure.status != "error" {
		t.Fatalf("landing failure status = %q, want error", failure.status)
	}
	if failure.err == nil || !strings.Contains(*failure.err, "crawl landing:") {
		t.Fatalf("landing failure error = %#v, want crawl landing error", failure.err)
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

	start := store.findRunWith("crawl", `"phase":"started"`)
	if start == nil {
		t.Fatalf("expected crawl start generation run, got %#v", store.runs)
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
	} else {
		output := string(run.output)
		for _, want := range []string{`"profile_stage":"full"`, `"duration_ms":`} {
			if !strings.Contains(output, want) {
				t.Fatalf("full profile run output = %s, want %s", output, want)
			}
		}
	}
	if len(provider.reqs) < 2 || !strings.Contains(provider.reqs[1].Prompt, "Article-specific proof") {
		t.Fatalf("full profile prompt did not include article corpus: %#v", provider.reqs)
	}
}

func TestPersistInventoryUsesBoundedParallelWorkers(t *testing.T) {
	release := make(chan struct{})
	provider := &blockingInventoryLLM{release: release}
	insight := NewInsight(Deps{Q: db.New(&insightDBSpy{}), LLM: provider}, nil)
	res := &crawl.Result{
		Articles: []*crawl.Page{
			{URL: "https://example.com/blog/a", Title: "A", Text: "A text"},
			{URL: "https://example.com/blog/b", Title: "B", Text: "B text"},
			{URL: "https://example.com/blog/c", Title: "C", Text: "C text"},
			{URL: "https://example.com/blog/d", Title: "D", Text: "D text"},
			{URL: "https://example.com/blog/e", Title: "E", Text: "E text"},
		},
	}

	done := make(chan int, 1)
	go func() {
		done <- insight.persistInventory(context.Background(), uuid.New(), res)
	}()

	if !provider.waitForStarted(2, 500*time.Millisecond) {
		close(release)
		<-done
		t.Fatal("inventory extraction did not start a second page while the first was still running")
	}
	close(release)

	count := <-done
	if count != len(res.Articles) {
		t.Fatalf("inventory count = %d, want %d", count, len(res.Articles))
	}
	if provider.maxActive < 2 {
		t.Fatalf("max active inventory calls = %d, want at least 2", provider.maxActive)
	}
	if provider.maxActive > inventoryWorkerLimit {
		t.Fatalf("max active inventory calls = %d, want <= %d", provider.maxActive, inventoryWorkerLimit)
	}
}

func TestPersistInventoryCapsEvidencePages(t *testing.T) {
	res := &crawl.Result{Articles: make([]*crawl.Page, 0, 22)}
	for i := 0; i < 22; i++ {
		res.Articles = append(res.Articles, &crawl.Page{
			URL:   "https://example.com/blog/page-" + string(rune('a'+i)),
			Title: "Page",
			Text:  "Evidence text",
		})
	}
	insight := NewInsight(Deps{Q: db.New(&insightDBSpy{}), LLM: &inventoryJSONLLM{}}, nil)

	count := insight.persistInventory(context.Background(), uuid.New(), res)

	if count != 20 {
		t.Fatalf("inventory count = %d, want 20", count)
	}
}

func TestPersistInventoryTimesOutSlowPagesAndKeepsSuccessfulEvidence(t *testing.T) {
	provider := &selectiveInventoryLLM{}
	insight := NewInsight(Deps{Q: db.New(&insightDBSpy{}), LLM: provider}, nil)
	insight.inventoryExtractionTimeout = 50 * time.Millisecond
	res := &crawl.Result{
		Articles: []*crawl.Page{
			{URL: "https://example.com/blog/slow", Title: "Slow", Text: "Slow text"},
			{URL: "https://example.com/blog/fast", Title: "Fast", Text: "Fast text"},
		},
	}

	count := insight.persistInventory(context.Background(), uuid.New(), res)

	if count != 1 {
		t.Fatalf("inventory count = %d, want 1 successful page", count)
	}
	if !provider.slowCancelled() {
		t.Fatal("slow inventory page was not cancelled by the per-page timeout")
	}
}

type capturedRun struct {
	input  []byte
	output []byte
	status string
	err    *string
	model  *string
	tokens *int32
	cost   pgtype.Numeric
}

type capturedProfileInsert struct {
	sourceURLs json.RawMessage
}

type blockingInventoryLLM struct {
	mu        sync.Mutex
	started   int
	active    int
	maxActive int
	release   <-chan struct{}
}

type inventoryJSONLLM struct{}

func (p *inventoryJSONLLM) Complete(ctx context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	return llm.CompletionResp{
		Text:   `{"title":"Inventory","target_keyword":"inventory","topics":["inventory"],"summary":"Summary","evidence_snippets":["UniPost schedules posts."]}`,
		Model:  "inventory-json-test",
		Tokens: 100,
	}, nil
}

type selectiveInventoryLLM struct {
	mu                 sync.Mutex
	slowCancelledByCtx bool
}

func (p *selectiveInventoryLLM) Complete(ctx context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	if strings.Contains(req.Prompt, "/slow") {
		<-ctx.Done()
		p.mu.Lock()
		p.slowCancelledByCtx = true
		p.mu.Unlock()
		return llm.CompletionResp{Model: "selective-test"}, ctx.Err()
	}
	return llm.CompletionResp{
		Text:   `{"title":"Fast","target_keyword":"fast","topics":["fast"],"summary":"Fast summary","evidence_snippets":["Fast article evidence."]}`,
		Model:  "selective-test",
		Tokens: 100,
	}, nil
}

func (p *selectiveInventoryLLM) slowCancelled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.slowCancelledByCtx
}

func (p *blockingInventoryLLM) Complete(ctx context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	p.mu.Lock()
	p.started++
	p.active++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.mu.Unlock()

	select {
	case <-p.release:
	case <-ctx.Done():
		return llm.CompletionResp{}, ctx.Err()
	}

	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return llm.CompletionResp{
		Text:   `{"title":"Inventory","target_keyword":"inventory","topics":["inventory"],"summary":"Summary","evidence_snippets":["UniPost schedules posts."]}`,
		Model:  "blocking-test",
		Tokens: 100,
	}, nil
}

func (p *blockingInventoryLLM) waitForStarted(want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		started := p.started
		p.mu.Unlock()
		if started >= want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
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
		output, _ := args[3].([]byte)
		model, _ := args[4].(*string)
		tokens, _ := args[5].(*int32)
		cost, _ := args[6].(pgtype.Numeric)
		s.runs = append(s.runs, capturedRun{input: input, output: output, status: status, err: errValue, model: model, tokens: tokens, cost: cost})
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
	for i := len(s.runs) - 1; i >= 0; i-- {
		if strings.Contains(string(s.runs[i].input), needle) {
			return &s.runs[i]
		}
	}
	return nil
}

func (s *insightDBSpy) findRunWith(step string, inputFragment string) *capturedRun {
	needle := `"step":"` + step + `"`
	for i := range s.runs {
		input := string(s.runs[i].input)
		if strings.Contains(input, needle) && strings.Contains(input, inputFragment) {
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
