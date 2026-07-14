package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Brave is a real SearchProvider backed by the Brave Search API.
// This is the default concrete behind the SearchProvider interface; swapping to
// Tavily/Serper/etc. is a single-file change (PRD §6: SearchProvider routing).
type Brave struct {
	APIKey string
	client *http.Client
	now    func() time.Time
}

func NewBrave(apiKey string) *Brave {
	return &Brave{APIKey: apiKey, client: &http.Client{Timeout: 15 * time.Second}, now: time.Now}
}

func (b *Brave) ProviderName() string { return "brave_web_search" }
func (b *Brave) Synthetic() bool      { return false }

type braveResp struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func (b *Brave) Search(ctx context.Context, q Query) ([]Result, error) {
	if b.APIKey == "" {
		return nil, fmt.Errorf("search api key not set")
	}
	count := q.Count
	if count <= 0 {
		count = 5
	}
	endpoint := "https://api.search.brave.com/res/v1/web/search?q=" +
		url.QueryEscape(q.Text) + "&count=" + strconv.Itoa(count)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", b.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("brave %d: %s", resp.StatusCode, string(raw))
	}
	var br braveResp
	if err := json.Unmarshal(raw, &br); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(br.Web.Results))
	for index, r := range br.Web.Results {
		out = append(out, Result{
			Title: r.Title, URL: r.URL, Snippet: r.Description,
			Source: "brave_web_search", FetchedAt: b.now(), ProviderOrder: index + 1,
		})
	}
	return out, nil
}
