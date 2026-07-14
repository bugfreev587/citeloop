package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/google/uuid"
)

const searchQuotaPerRun = 10 // §5.2: default ≤ 10 search calls per topic run

// Strategist does gap analysis + keyword/AI-prompt research (via SearchProvider)
// and produces a topic backlog (PRD §5.2). Search failures degrade to pure-LLM
// selection with degraded=true rather than failing the whole run.
type Strategist struct {
	Deps
	Log *slog.Logger
}

func NewStrategist(d Deps, log *slog.Logger) *Strategist {
	if log == nil {
		log = slog.Default()
	}
	return &Strategist{Deps: d, Log: log}
}

// Run produces and persists topics for a project. Returns the created topics.
func (a *Strategist) Run(ctx context.Context, projectID uuid.UUID, cfg config.ProjectConfig) ([]db.Topic, error) {
	profile, err := a.Q.GetActiveProfile(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("no active profile: %w", err)
	}
	inv, err := a.Q.ListInventory(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// --- research phase (bounded, degradable) ---
	var snapshots []search.Result
	degraded := false
	queries := a.seedQueries(profile.Profile)
	for i, q := range queries {
		if i >= searchQuotaPerRun {
			break
		}
		results, serr := a.Search.Search(ctx, search.Query{Text: q, Count: 5})
		if serr != nil {
			a.Log.Warn("search failed, degrading to pure-LLM", "query", q, "err", serr)
			degraded = true
			break
		}
		snapshots = append(snapshots, results...)
	}

	// --- topic generation ---
	existing := make([]string, 0, len(inv))
	for _, it := range inv {
		title := ""
		if it.Title != nil {
			title = *it.Title
		}
		existing = append(existing, title+" — "+it.Url)
	}
	specs, resp, gerr := a.generateTracked(ctx, projectID, profile.Profile, existing, snapshots, cfg)
	specs = normalizeTopicSpecs(specs)

	out := map[string]any{"degraded": degraded, "search": snapshots, "topics": specs}
	recordRun(ctx, a.Q, projectID, agentStrategist,
		map[string]any{"queries": queries}, out, resp, gerr)
	if gerr != nil {
		return nil, gerr
	}

	created := make([]db.Topic, 0, len(specs))
	for _, s := range specs {
		channel := normalizeChannel(s.Channel)
		t, err := a.Q.CreateTopic(ctx, db.CreateTopicParams{
			ProjectID:     projectID,
			Channel:       channel,
			Title:         s.Title,
			TargetKeyword: ptr(s.TargetKeyword),
			TargetPrompt:  ptr(s.TargetPrompt),
			Angle:         ptr(s.Angle),
			Format:        ptr(s.Format),
			Priority:      int32(s.Priority),
			InternalLinks: toJSON(s.InternalLinks),
			Status:        string(topicstate.StatusBacklog),
		})
		if err != nil {
			a.Log.Warn("create topic failed", "title", s.Title, "err", err)
			continue
		}
		created = append(created, t)
	}
	a.Log.Info("strategist complete", "topics", len(created), "degraded", degraded)
	return created, nil
}

func (a *Strategist) seedQueries(profileJSON json.RawMessage) []string {
	profileJSON = growthradar.DiscoveryProfile(profileJSON, growthradar.EvidenceIndex{})
	var p Profile
	var sanitized struct {
		Accepted    []string `json:"accepted_public_vocabulary"`
		Competitors []string `json:"confirmed_competitors"`
	}
	_ = json.Unmarshal(profileJSON, &sanitized)
	p.KeyTerms, p.Competitors = sanitized.Accepted, sanitized.Competitors
	var qs []string
	for _, k := range p.KeyTerms {
		qs = append(qs, k+" best tools 2026")
	}
	for _, c := range p.Competitors {
		qs = append(qs, c+" alternative")
	}
	if len(qs) == 0 {
		qs = []string{p.Positioning}
	}
	return qs
}

func (a *Strategist) generate(ctx context.Context, profileJSON json.RawMessage, existing []string, snaps []search.Result, cfg config.ProjectConfig) ([]TopicSpec, llm.CompletionResp, error) {
	return a.generateTracked(ctx, uuid.Nil, profileJSON, existing, snaps, cfg)
}

func (a *Strategist) generateTracked(ctx context.Context, projectID uuid.UUID, profileJSON json.RawMessage, existing []string, snaps []search.Result, cfg config.ProjectConfig) ([]TopicSpec, llm.CompletionResp, error) {
	profileJSON = growthradar.DiscoveryProfile(profileJSON, growthradar.EvidenceIndex{})
	searchCtx := ""
	for i, s := range snaps {
		if i >= 20 {
			break
		}
		searchCtx += fmt.Sprintf("- %s (%s): %s\n", s.Title, s.URL, s.Snippet)
	}
	prompt := fmt.Sprintf(`[[STRATEGIST]] Produce a content topic backlog via gap analysis.
Generate at least 10 topics not duplicating existing content.
Channel mix target: blog %.0f%%, syndication %.0f%%. Use channel ∈ {blog, syndication, both}.
Return JSON: {"topics":[{channel,title,target_keyword,target_prompt,angle,format,priority,internal_links[]}]}.

PRODUCT PROFILE:
%s

EXISTING CONTENT (avoid duplicates):
%s

SEARCH SIGNALS:
%s`, cfg.ChannelMix.Blog*100, cfg.ChannelMix.Syndication*100,
		clip(string(profileJSON), 3000), strings.Join(existing, "\n"), searchCtx)

	resp, callID, err := completeTracked(ctx, a.AICalls, a.LLM, projectID, "growth_hypothesis", "project", projectID, "growth-strategist-v2", uuid.Nil, uuid.Nil, llm.CompletionReq{
		System: "You are an SEO+GEO content strategist.",
		Prompt: prompt, JSON: true, MaxTokens: 3000,
	})
	if err != nil {
		return nil, resp, err
	}
	var wrap struct {
		Topics []TopicSpec `json:"topics"`
	}
	if err := extractJSON(resp.Text, &wrap); err != nil {
		ledgerErr := failTrackedOutput(ctx, a.AICalls, projectID, callID, "invalid_response")
		return nil, resp, fmt.Errorf("parse topics: %w", errors.Join(err, ledgerErr))
	}
	return wrap.Topics, resp, nil
}

func normalizeChannel(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "blog", "syndication", "both":
		return strings.ToLower(c)
	default:
		return "blog"
	}
}

func normalizeTopicSpecs(specs []TopicSpec) []TopicSpec {
	for i := range specs {
		if specs[i].Priority <= 0 {
			specs[i].Priority = fallbackTopicPriority(i)
		}
		if specs[i].Priority > 10 {
			specs[i].Priority = normalizeTopicPriorityNumber(float64(specs[i].Priority))
		}
	}
	return specs
}

func fallbackTopicPriority(index int) int {
	priority := index + 1
	if priority > 10 {
		return 10
	}
	return priority
}
