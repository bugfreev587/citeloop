package googledata

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchConsoleFetchParsesDailyPageQueryAndAppearanceRows(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		seen = append(seen, r.URL.Path)
		body := readBody(t, r)
		switch {
		case strings.Contains(body, `"searchAppearance"`):
			w.Write([]byte(`{"rows":[{"keys":["2026-06-01","WEB_RESULT"],"clicks":3,"impressions":30,"ctr":0.1,"position":4.5}]}`))
		case strings.Contains(body, `"query"`):
			w.Write([]byte(`{"rows":[{"keys":["2026-06-01","https://unipost.dev/blog/a","best scheduler","usa","DESKTOP"],"clicks":2,"impressions":20,"ctr":0.1,"position":8}]}`))
		default:
			w.Write([]byte(`{"rows":[{"keys":["2026-06-01","https://unipost.dev/blog/a"],"clicks":5,"impressions":50,"ctr":0.1,"position":6}]}`))
		}
	}))
	defer srv.Close()

	client := Client{HTTPClient: srv.Client(), SearchConsoleBaseURL: srv.URL + "/webmasters/v3"}
	data, err := client.FetchSearchConsole(context.Background(), SearchConsoleRequest{
		SiteURL:   "sc-domain:unipost.dev",
		StartDate: date(2026, 6, 1),
		EndDate:   date(2026, 6, 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 {
		t.Fatalf("requests = %d, want 3", len(seen))
	}
	if got := data.PageRows[0].PageURL; got != "https://unipost.dev/blog/a" {
		t.Fatalf("page url = %q", got)
	}
	if got := data.QueryRows[0].Query; got != "best scheduler" {
		t.Fatalf("query = %q", got)
	}
	if got := data.AppearanceRows[0].SearchAppearance; got != "WEB_RESULT" {
		t.Fatalf("appearance = %q", got)
	}
}

func TestSearchConsoleFetchKeepsPageAndQueryRowsWhenAppearanceUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		switch {
		case strings.Contains(body, `"searchAppearance"`):
			http.Error(w, `{"error":{"code":400,"message":"Cannot group by search appearance dimension together with another dimension.","reason":"invalidParameter"}}`, http.StatusBadRequest)
		case strings.Contains(body, `"query"`):
			w.Write([]byte(`{"rows":[{"keys":["2026-06-01","https://unipost.dev/blog/a","best scheduler","usa","DESKTOP"],"clicks":2,"impressions":20,"ctr":0.1,"position":8}]}`))
		default:
			w.Write([]byte(`{"rows":[{"keys":["2026-06-01","https://unipost.dev/blog/a"],"clicks":5,"impressions":50,"ctr":0.1,"position":6}]}`))
		}
	}))
	defer srv.Close()

	client := Client{HTTPClient: srv.Client(), SearchConsoleBaseURL: srv.URL + "/webmasters/v3"}
	data, err := client.FetchSearchConsole(context.Background(), SearchConsoleRequest{
		SiteURL:   "sc-domain:unipost.dev",
		StartDate: date(2026, 6, 1),
		EndDate:   date(2026, 6, 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.PageRows) != 1 || len(data.QueryRows) != 1 {
		t.Fatalf("page/query rows = %d/%d, want 1/1", len(data.PageRows), len(data.QueryRows))
	}
	if len(data.AppearanceRows) != 0 {
		t.Fatalf("appearance rows = %d, want 0", len(data.AppearanceRows))
	}
}

func TestAnalyticsFetchParsesLandingPageRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/properties/123456:runReport") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"rows":[{"dimensionValues":[{"value":"20260601"},{"value":"/blog/a"}],"metricValues":[{"value":"10"},{"value":"7"},{"value":"1"}]}]}`))
	}))
	defer srv.Close()

	client := Client{HTTPClient: srv.Client(), AnalyticsDataBaseURL: srv.URL + "/v1beta"}
	rows, err := client.FetchAnalytics(context.Background(), AnalyticsRequest{
		PropertyID: "123456",
		StartDate:  date(2026, 6, 1),
		EndDate:    date(2026, 6, 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].PagePath != "/blog/a" || rows[0].Sessions != 10 || rows[0].EngagedSessions != 7 || rows[0].KeyEvents != 1 {
		t.Fatalf("row = %#v", rows[0])
	}
}

func TestListSearchConsoleSitesParsesAuthorizedProperties(t *testing.T) {
	var seenMethod string
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		w.Write([]byte(`{"siteEntry":[{"siteUrl":"sc-domain:unipost.dev","permissionLevel":"siteOwner"},{"siteUrl":"https://unipost.dev/","permissionLevel":"siteFullUser"}]}`))
	}))
	defer srv.Close()

	client := Client{HTTPClient: srv.Client(), SearchConsoleBaseURL: srv.URL + "/webmasters/v3"}
	sites, err := client.ListSearchConsoleSites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if seenMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", seenMethod)
	}
	if seenPath != "/webmasters/v3/sites" {
		t.Fatalf("path = %s", seenPath)
	}
	if len(sites) != 2 {
		t.Fatalf("sites = %d, want 2", len(sites))
	}
	if sites[0].SiteURL != "sc-domain:unipost.dev" || sites[0].PermissionLevel != "siteOwner" {
		t.Fatalf("site = %#v", sites[0])
	}
}

func TestSearchConsoleOAuthConfigUsesReadonlyScopes(t *testing.T) {
	cfg := SearchConsoleOAuthConfig("client-id", "client-secret", "https://app.citeloop.test/callback")
	if cfg.ClientID != "client-id" || cfg.ClientSecret != "client-secret" {
		t.Fatalf("client config = %#v", cfg)
	}
	if cfg.RedirectURL != "https://app.citeloop.test/callback" {
		t.Fatalf("redirect URL = %q", cfg.RedirectURL)
	}
	if len(cfg.Scopes) != 2 || cfg.Scopes[0] != ScopeSearchConsoleReadonly || cfg.Scopes[1] != ScopeAnalyticsReadonly {
		t.Fatalf("scopes = %#v", cfg.Scopes)
	}
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
