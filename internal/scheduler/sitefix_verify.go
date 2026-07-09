package scheduler

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
)

const (
	// siteFixDeployGrace is how long to wait after merge before the first
	// production check, giving the host time to redeploy.
	siteFixDeployGrace = 3 * time.Minute
	// siteFixVerifyDeadline bounds the URL re-check for verifiable fixes; past it
	// the change is flagged for manual follow-up instead of silently "verified".
	siteFixVerifyDeadline = 24 * time.Hour
	// siteFixFuzzyVerifyGrace is when a non-URL-verifiable fix auto-advances to
	// verification after merge (operator's chosen policy).
	siteFixFuzzyVerifyGrace = 1 * time.Hour
	// siteFixVerifyMaxBody caps the bytes read from a target page.
	siteFixVerifyMaxBody = 2 << 20 // 2MB
)

// reconcileSiteChangeVerificationForProject advances merged applications toward
// verified. metadata rewrites are confirmed by re-fetching the target URL and
// checking the proposed title/meta are live; other (fuzzy) fixes auto-verify a
// grace period after merge since they cannot be checked programmatically.
func (s *Scheduler) reconcileSiteChangeVerificationForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	apps, err := q.ListMergedSiteChangeApplicationsForVerification(ctx, p.ID)
	if err != nil {
		return err
	}
	now := s.currentTime()
	for _, app := range apps {
		merged := siteFixMergedAt(app)
		elapsed := now.Sub(merged)

		if !siteFixURLVerifiable(app) {
			// Fuzzy fix: nothing to fetch. Auto-advance to verification a grace
			// period after merge, per the operator's policy.
			if elapsed >= siteFixFuzzyVerifyGrace {
				if err := s.markSiteChangeVerified(ctx, q, p, app, "auto_grace_period", map[string]any{
					"reason":       "non-URL-verifiable fix auto-verified after merge grace period",
					"grace_period": siteFixFuzzyVerifyGrace.String(),
				}); err != nil {
					s.Log.Error("auto-verify fuzzy site fix", "project", p.ID, "application", app.ID, "err", err)
				}
			}
			continue
		}

		// URL-verifiable fix (metadata rewrite): re-check on a backoff.
		if !siteFixTimeDue(app.NextPollAt, now) {
			continue
		}
		targetURL := strings.TrimSpace(app.TargetUrl)
		title, meta := siteFixProposedMetadata(app)
		matched := false
		if targetURL != "" && (title != "" || meta != "") {
			pageHTML, ferr := s.fetchSiteFixPage(ctx, targetURL)
			if ferr != nil {
				s.Log.Warn("fetch site fix target for verification", "project", p.ID, "application", app.ID, "url", targetURL, "err", ferr)
			} else {
				matched = pageEmitsProposedMetadata(pageHTML, title, meta)
			}
		}

		if matched {
			if err := s.markSiteChangeVerified(ctx, q, p, app, "auto_url_check", map[string]any{
				"reason":     "target URL now emits the proposed title and meta description",
				"target_url": targetURL,
			}); err != nil {
				s.Log.Error("verify site fix via URL", "project", p.ID, "application", app.ID, "err", err)
			}
			continue
		}

		if elapsed >= siteFixVerifyDeadline {
			if err := s.markSiteChangePRNeedsFollowUp(ctx, q, p, app, "merged but the change was not detected live within 24h — check the deploy"); err != nil {
				s.Log.Error("flag unverified site fix", "project", p.ID, "application", app.ID, "err", err)
			}
			continue
		}

		if next, ok := nextSiteFixVerifyPollAt(merged, now); ok {
			if err := q.SetSiteChangePRNextPollAt(ctx, db.SetSiteChangePRNextPollAtParams{
				ID:         app.ID,
				ProjectID:  p.ID,
				NextPollAt: pgutil.TS(next),
			}); err != nil {
				s.Log.Error("schedule next site fix verification check", "project", p.ID, "application", app.ID, "err", err)
			}
		}
	}
	return nil
}

// nextSiteFixVerifyPollAt reuses the Fibonacci checkpoints (anchored at merge) to
// re-check the live URL densely right after deploy, until the 24h deadline.
func nextSiteFixVerifyPollAt(mergedAt, now time.Time) (time.Time, bool) {
	elapsed := now.Sub(mergedAt)
	if elapsed >= siteFixVerifyDeadline {
		return time.Time{}, false
	}
	for _, cp := range siteFixPRPollCheckpoints {
		if cp > elapsed {
			return mergedAt.Add(cp), true
		}
	}
	return now.Add(siteFixPRDailyInterval), true
}

func siteFixMergedAt(app db.SiteChangeApplication) time.Time {
	if app.MergedAt.Valid {
		return app.MergedAt.Time
	}
	return siteFixPRCreatedAt(app)
}

// siteFixURLVerifiable reports whether the fix can be confirmed by fetching the
// target URL (metadata rewrites); other asset types are treated as fuzzy.
func siteFixURLVerifiable(app db.SiteChangeApplication) bool {
	return siteFixAssetType(app) == "metadata_rewrite"
}

func siteFixAssetType(app db.SiteChangeApplication) string {
	if at := jsonStringField(app.ResolutionCriteria, "asset_type"); at != "" {
		return at
	}
	return jsonStringField(app.PatchSnapshot, "asset_type")
}

// siteFixProposedMetadata reads the proposed title and meta description the PR
// was supposed to ship, from the patch snapshot (with a diff-snapshot fallback).
func siteFixProposedMetadata(app db.SiteChangeApplication) (title, meta string) {
	title, meta = proposedMetadataFrom(app.PatchSnapshot, "proposed_change")
	if title == "" && meta == "" {
		title, meta = proposedMetadataFrom(app.DiffSnapshot, "proposed_metadata")
	}
	return title, meta
}

func proposedMetadataFrom(raw json.RawMessage, key string) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", ""
	}
	inner, ok := obj[key]
	if !ok {
		return "", ""
	}
	var change struct {
		Title           string `json:"title"`
		MetaDescription string `json:"meta_description"`
	}
	if err := json.Unmarshal(inner, &change); err != nil {
		return "", ""
	}
	return strings.TrimSpace(change.Title), strings.TrimSpace(change.MetaDescription)
}

func jsonStringField(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	v, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// pageEmitsProposedMetadata reports whether the fetched HTML contains the
// proposed title and meta description. Empty proposed values are ignored; both
// sides are normalised (HTML-unescaped, whitespace-collapsed, lowercased) so
// entity encoding or spacing differences do not cause false negatives.
func pageEmitsProposedMetadata(pageHTML, title, meta string) bool {
	normalized := normalizeForMatch(pageHTML)
	if title != "" && !strings.Contains(normalized, normalizeForMatch(title)) {
		return false
	}
	if meta != "" && !strings.Contains(normalized, normalizeForMatch(meta)) {
		return false
	}
	// Guard against "both empty" being treated as a trivial match.
	return title != "" || meta != ""
}

func normalizeForMatch(s string) string {
	s = html.UnescapeString(s)
	s = strings.ToLower(s)
	return strings.Join(strings.Fields(s), " ")
}

func (s *Scheduler) fetchSiteFixPage(ctx context.Context, url string) (string, error) {
	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "CiteLoopVerifier/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", &siteFixHTTPError{status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, siteFixVerifyMaxBody))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type siteFixHTTPError struct{ status int }

func (e *siteFixHTTPError) Error() string {
	return "target returned status " + http.StatusText(e.status)
}

func (s *Scheduler) markSiteChangeVerified(ctx context.Context, q *db.Queries, p db.Project, app db.SiteChangeApplication, source string, detail map[string]any) error {
	now := s.currentTime()
	snapshot := map[string]any{"source": source, "verified_at": now.UTC().Format(time.RFC3339)}
	for k, v := range detail {
		snapshot[k] = v
	}
	snapRaw := json.RawMessage(mustJSON(snapshot))
	if _, err := q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
		ID:                   app.ID,
		ProjectID:            p.ID,
		Status:               "verified",
		DeploymentSnapshot:   json.RawMessage(`{}`),
		VerificationSnapshot: snapRaw,
	}); err != nil {
		return err
	}
	// Advance the content action into the measurement loop (same transition the
	// manual "Mark applied" path uses), so the fix enters attribution.
	if _, err := q.MarkContentActionVerification(ctx, db.MarkContentActionVerificationParams{
		ID:                   app.ContentActionID,
		ProjectID:            p.ID,
		Status:               "measuring",
		VerifiedAt:           pgutil.TS(now),
		VerificationSnapshot: snapRaw,
	}); err != nil {
		return err
	}
	s.Log.Info("site fix verified", "project", p.ID, "application", app.ID, "source", source, "target_url", app.TargetUrl)
	return nil
}
