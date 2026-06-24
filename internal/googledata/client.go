// Package googledata reads Google Search Console and GA4 Data API metrics.
package googledata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSearchConsoleBaseURL = "https://www.googleapis.com/webmasters/v3"
	defaultAnalyticsDataBaseURL = "https://analyticsdata.googleapis.com/v1beta"
)

type Client struct {
	HTTPClient           *http.Client
	SearchConsoleBaseURL string
	AnalyticsDataBaseURL string
}

type SearchConsoleRequest struct {
	SiteURL   string
	StartDate time.Time
	EndDate   time.Time
	RowLimit  int
}

type SearchConsoleData struct {
	PageRows       []SearchConsolePageRow
	QueryRows      []SearchConsoleQueryRow
	AppearanceRows []SearchConsoleAppearanceRow
}

type SearchConsolePageRow struct {
	Date        time.Time
	PageURL     string
	Clicks      float64
	Impressions float64
	CTR         float64
	Position    float64
}

type SearchConsoleQueryRow struct {
	Date        time.Time
	PageURL     string
	Query       string
	Country     string
	Device      string
	Clicks      float64
	Impressions float64
	CTR         float64
	Position    float64
}

type SearchConsoleAppearanceRow struct {
	Date             time.Time
	SearchAppearance string
	Clicks           float64
	Impressions      float64
	CTR              float64
	Position         float64
}

type SearchConsoleSite struct {
	SiteURL         string `json:"siteUrl"`
	PermissionLevel string `json:"permissionLevel"`
}

type AnalyticsRequest struct {
	PropertyID string
	StartDate  time.Time
	EndDate    time.Time
	RowLimit   int
}

type AnalyticsPageRow struct {
	Date            time.Time
	PagePath        string
	Sessions        float64
	EngagedSessions float64
	KeyEvents       float64
}

type searchAnalyticsResponse struct {
	Rows []searchAnalyticsRow `json:"rows"`
}

type searchAnalyticsRow struct {
	Keys        []string `json:"keys"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
	CTR         float64  `json:"ctr"`
	Position    float64  `json:"position"`
}

type searchConsoleSitesResponse struct {
	SiteEntry []SearchConsoleSite `json:"siteEntry"`
}

func (c Client) ListSearchConsoleSites(ctx context.Context) ([]SearchConsoleSite, error) {
	var out searchConsoleSitesResponse
	if err := c.getJSON(ctx, c.searchConsoleBaseURL()+"/sites", &out); err != nil {
		return nil, err
	}
	return out.SiteEntry, nil
}

func (c Client) FetchSearchConsole(ctx context.Context, req SearchConsoleRequest) (SearchConsoleData, error) {
	var out SearchConsoleData
	pages, err := c.searchAnalytics(ctx, req, []string{"date", "page"})
	if err != nil {
		return out, err
	}
	for _, row := range pages.Rows {
		if len(row.Keys) < 2 {
			continue
		}
		out.PageRows = append(out.PageRows, SearchConsolePageRow{
			Date:        parseGSCDate(row.Keys[0]),
			PageURL:     row.Keys[1],
			Clicks:      row.Clicks,
			Impressions: row.Impressions,
			CTR:         row.CTR,
			Position:    row.Position,
		})
	}
	queryRows, err := c.searchAnalytics(ctx, req, []string{"date", "page", "query", "country", "device"})
	if err != nil {
		return out, err
	}
	for _, row := range queryRows.Rows {
		if len(row.Keys) < 5 {
			continue
		}
		out.QueryRows = append(out.QueryRows, SearchConsoleQueryRow{
			Date:        parseGSCDate(row.Keys[0]),
			PageURL:     row.Keys[1],
			Query:       row.Keys[2],
			Country:     row.Keys[3],
			Device:      row.Keys[4],
			Clicks:      row.Clicks,
			Impressions: row.Impressions,
			CTR:         row.CTR,
			Position:    row.Position,
		})
	}
	appearances, err := c.searchAnalytics(ctx, req, []string{"date", "searchAppearance"})
	if err != nil {
		if isUnsupportedSearchAppearanceError(err) {
			return out, nil
		}
		return out, err
	}
	for _, row := range appearances.Rows {
		if len(row.Keys) < 2 {
			continue
		}
		out.AppearanceRows = append(out.AppearanceRows, SearchConsoleAppearanceRow{
			Date:             parseGSCDate(row.Keys[0]),
			SearchAppearance: row.Keys[1],
			Clicks:           row.Clicks,
			Impressions:      row.Impressions,
			CTR:              row.CTR,
			Position:         row.Position,
		})
	}
	return out, nil
}

func isUnsupportedSearchAppearanceError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "search appearance dimension") && strings.Contains(msg, "invalidparameter")
}

func (c Client) searchAnalytics(ctx context.Context, req SearchConsoleRequest, dimensions []string) (searchAnalyticsResponse, error) {
	rowLimit := req.RowLimit
	if rowLimit <= 0 {
		rowLimit = 25000
	}
	body := map[string]any{
		"startDate":  formatDate(req.StartDate),
		"endDate":    formatDate(req.EndDate),
		"dimensions": dimensions,
		"rowLimit":   rowLimit,
		"dataState":  "final",
	}
	var out searchAnalyticsResponse
	endpoint := fmt.Sprintf("%s/sites/%s/searchAnalytics/query", c.searchConsoleBaseURL(), url.PathEscape(req.SiteURL))
	if err := c.postJSON(ctx, endpoint, body, &out); err != nil {
		return out, err
	}
	return out, nil
}

type analyticsReportResponse struct {
	Rows []analyticsReportRow `json:"rows"`
}

type analyticsReportRow struct {
	DimensionValues []analyticsValue `json:"dimensionValues"`
	MetricValues    []analyticsValue `json:"metricValues"`
}

type analyticsValue struct {
	Value string `json:"value"`
}

func (c Client) FetchAnalytics(ctx context.Context, req AnalyticsRequest) ([]AnalyticsPageRow, error) {
	rowLimit := req.RowLimit
	if rowLimit <= 0 {
		rowLimit = 25000
	}
	body := map[string]any{
		"dateRanges": []map[string]string{{
			"startDate": formatDate(req.StartDate),
			"endDate":   formatDate(req.EndDate),
		}},
		"dimensions": []map[string]string{
			{"name": "date"},
			{"name": "landingPagePlusQueryString"},
		},
		"metrics": []map[string]string{
			{"name": "sessions"},
			{"name": "engagedSessions"},
			{"name": "keyEvents"},
		},
		"limit": fmt.Sprintf("%d", rowLimit),
	}
	var res analyticsReportResponse
	id := strings.TrimPrefix(strings.TrimSpace(req.PropertyID), "properties/")
	endpoint := fmt.Sprintf("%s/properties/%s:runReport", c.analyticsDataBaseURL(), url.PathEscape(id))
	if err := c.postJSON(ctx, endpoint, body, &res); err != nil {
		return nil, err
	}
	rows := make([]AnalyticsPageRow, 0, len(res.Rows))
	for _, row := range res.Rows {
		if len(row.DimensionValues) < 2 || len(row.MetricValues) < 3 {
			continue
		}
		rows = append(rows, AnalyticsPageRow{
			Date:            parseGA4Date(row.DimensionValues[0].Value),
			PagePath:        row.DimensionValues[1].Value,
			Sessions:        parseFloat(row.MetricValues[0].Value),
			EngagedSessions: parseFloat(row.MetricValues[1].Value),
			KeyEvents:       parseFloat(row.MetricValues[2].Value),
		})
	}
	return rows, nil
}

func (c Client) postJSON(ctx context.Context, endpoint string, payload any, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("google api status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func (c Client) getJSON(ctx context.Context, endpoint string, out any) error {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("google api status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func (c Client) searchConsoleBaseURL() string {
	if strings.TrimSpace(c.SearchConsoleBaseURL) != "" {
		return strings.TrimRight(c.SearchConsoleBaseURL, "/")
	}
	return defaultSearchConsoleBaseURL
}

func (c Client) analyticsDataBaseURL() string {
	if strings.TrimSpace(c.AnalyticsDataBaseURL) != "" {
		return strings.TrimRight(c.AnalyticsDataBaseURL, "/")
	}
	return defaultAnalyticsDataBaseURL
}

func formatDate(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func parseGSCDate(value string) time.Time {
	t, _ := time.Parse("2006-01-02", value)
	return t
}

func parseGA4Date(value string) time.Time {
	t, _ := time.Parse("20060102", value)
	return t
}

func parseFloat(value string) float64 {
	f, _ := strconv.ParseFloat(value, 64)
	return f
}
