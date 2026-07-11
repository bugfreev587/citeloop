package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	xhtml "golang.org/x/net/html"
)

const (
	// siteFixDeployGrace is how long to wait after merge before the first
	// production check, giving the host time to redeploy.
	siteFixDeployGrace = 3 * time.Minute
	// siteFixVerifyDeadline bounds the URL re-check for verifiable fixes; past it
	// the change is flagged for manual follow-up instead of silently "verified".
	siteFixVerifyDeadline = 24 * time.Hour
	// siteFixVerifyMaxBody caps the bytes read from a target page.
	siteFixVerifyMaxBody = 2 << 20 // 2MB
)

// reconcileSiteChangeVerificationForProject advances merged applications toward
// verified. metadata rewrites are confirmed by re-fetching the target URL and
// checking the proposed title/meta are live; other (fuzzy) fixes auto-verify a
// grace period after merge since they cannot be checked programmatically.
func (s *Scheduler) reconcileSiteChangeVerificationForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	if err := s.reconcileCanonicalSiteFixVerificationForProject(ctx, q, p); err != nil {
		return err
	}
	return s.reconcileLegacySiteChangeVerificationForProject(ctx, q, p)
}

// reconcileLegacySiteChangeVerificationForProject is a compatibility reader
// for unmigrated Content Action applications. Canonical Doctor work never
// reaches this path because its source is site_fix_id only.
func (s *Scheduler) reconcileLegacySiteChangeVerificationForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	apps, err := q.ListMergedSiteChangeApplicationsForVerification(ctx, p.ID)
	if err != nil {
		return err
	}
	now := s.currentTime()
	for _, app := range apps {
		merged := siteFixMergedAt(app)
		elapsed := now.Sub(merged)

		if !siteFixURLVerifiable(app) {
			// Legacy fuzzy applications are intentionally left open for manual
			// review. PR merge alone is never verification evidence.
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

func (s *Scheduler) reconcileCanonicalSiteFixVerificationForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	activeMarkerSiteFixes, err := q.ListDoctorAIOnDemandActiveSiteFixes(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, siteFixID := range activeMarkerSiteFixes {
		if _, err := q.RejectUnauthorizedDoctorAIOnDemandTriggers(ctx, db.RejectUnauthorizedDoctorAIOnDemandTriggersParams{ProjectID: p.ID, SiteFixID: siteFixID}); err != nil {
			return err
		}
	}
	rejectedRunningCalls, err := q.ListRejectedDoctorAIRunningCalls(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, callID := range rejectedRunningCalls {
		if !callID.Valid {
			continue
		}
		errorCode := "doctor_ai_marker_rejected"
		if _, err := q.FinishAICallRecordIfRunning(ctx, db.FinishAICallRecordIfRunningParams{ErrorCode: &errorCode, ID: uuid.UUID(callID.Bytes), ProjectID: p.ID}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	legacyMarkers, err := q.ListDoctorAIOnDemandConsumedUnapplied(ctx, p.ID)
	if err != nil {
		return err
	}
	for i := range legacyMarkers {
		marker := legacyMarkers[i]
		if marker.HasLifecycleReference {
			if _, err := q.MarkDoctorAIOnDemandLifecycleApplied(ctx, db.MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: marker.ProjectID, SiteFixID: marker.SiteFixID, RequestID: marker.RequestID, AiCallID: marker.AiCallID}); err != nil {
				return err
			}
		} else if _, err := q.RejectDoctorAIOnDemandConsumedWithoutLifecycleReference(ctx, db.RejectDoctorAIOnDemandConsumedWithoutLifecycleReferenceParams{ProjectID: marker.ProjectID, SiteFixID: marker.SiteFixID, RequestID: marker.RequestID, AiCallID: marker.AiCallID}); err != nil {
			return fmt.Errorf("reject consumed Doctor AI marker lifecycle_completed_without_this_ai_result: %w", err)
		}
	}
	apps, err := q.ListCanonicalSiteFixesForVerification(ctx, p.ID)
	if err != nil {
		return err
	}
	projectConfig, err := config.Parse(p.Config)
	if err != nil {
		return err
	}
	automaticAIAuthorized := projectConfig.AllowsDoctorAI(config.DoctorAITriggerVerificationScheduler)
	now := s.currentTime()
	for _, app := range apps {
		if !app.SiteFixID.Valid || app.ContentActionID.Valid {
			continue
		}
		fixID := uuid.UUID(app.SiteFixID.Bytes)
		fix, err := q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: p.ID})
		if err != nil {
			return err
		}
		if !projectConfig.AllowsDoctorAI(config.DoctorAITriggerVerificationUser) {
			if err := rejectCanonicalAIMarkers(ctx, q, p.ID, fix, canonicalPageEvidence{}, "Doctor AI policy is disabled"); err != nil {
				return err
			}
		}
		var carriedMarker *db.DoctorAiOnDemandTrigger
		if marker, markerErr := q.GetDoctorAIOnDemandConsumedResult(ctx, db.GetDoctorAIOnDemandConsumedResultParams{ProjectID: p.ID, SiteFixID: fix.ID}); markerErr == nil {
			carriedMarker = &marker
		} else if !errors.Is(markerErr, pgx.ErrNoRows) {
			return markerErr
		}
		if fix.Status == "failed_retryable" {
			// Retry is explicit: the active signature remains reserved until an
			// operator/user reopens the verification attempt.
			if err := rejectCanonicalAIMarkers(ctx, q, p.ID, fix, canonicalPageEvidence{}, "verification lifecycle was not reopened"); err != nil {
				return err
			}
			if err := markCanonicalAIReviewAppliedStrict(ctx, q, carriedMarker); err != nil {
				return err
			}
			continue
		}
		if !siteFixTimeDue(app.NextPollAt, now) {
			continue
		}
		targetURL := strings.TrimSpace(app.TargetUrl)
		page, fetchErr := s.fetchCanonicalSiteFixPage(ctx, projectConfig.SiteURL, targetURL)
		if fetchErr != nil {
			if err := rejectCanonicalAIMarkers(ctx, q, p.ID, fix, page, "production PageEvidence fetch failed"); err != nil {
				return err
			}
			if canonicalSiteFixVerifyExpired(app, now) {
				if err := s.recordCanonicalVerificationFailure(ctx, p.ID, fix, app, "target_fetch_failed", fetchErr.Error(), canonicalPageEvidenceMap(targetURL, page, nil), pgtype.UUID{}, carriedMarker); err != nil {
					return err
				}
			} else if err := s.scheduleCanonicalVerificationPollWithMarker(ctx, q, p.ID, app, now, carriedMarker); err != nil {
				return err
			}
			continue
		}

		results, passed, executable := evaluateCanonicalAcceptanceTests(fix, app, page)
		if executable {
			if err := rejectCanonicalAIMarkers(ctx, q, p.ID, fix, page, "deterministic acceptance evidence is sufficient"); err != nil {
				return err
			}
		}
		if !executable {
			acquired, acquireErr := s.acquireCanonicalAIReview(ctx, q, p.ID, fix, page, projectConfig, automaticAIAuthorized)
			if acquireErr != nil {
				return acquireErr
			}
			if acquired.Allowed {
				review, reviewErr := acquired.Review, acquired.ReviewErr
				callID := pgtype.UUID{Bytes: review.CallID, Valid: review.CallID != uuid.Nil}
				if reviewErr == nil && review.Decision == "passed" && review.Confidence >= 0.9 {
					if err := s.markCanonicalDeploymentObserved(ctx, q, p.ID, &fix, &app, targetURL, page, "authorized_ai_verification"); err != nil {
						return err
					}
					var aiResults any
					_ = json.Unmarshal(review.AcceptanceResults, &aiResults)
					evidence := canonicalPageEvidenceMap(targetURL, page, aiResults)
					evidence["source"], evidence["confidence"] = "authorized_ai_verification", review.Confidence
					if err := s.recordCanonicalVerificationPass(ctx, p.ID, fix, app, evidence, callID, acquired.Marker); err != nil {
						return err
					}
					continue
				}
				reason := "authorized AI verification was inconclusive or below the confidence threshold"
				if reviewErr != nil {
					reason = reviewErr.Error()
				}
				if canonicalSiteFixVerifyExpired(app, now) {
					evidence := canonicalPageEvidenceMap(targetURL, page, results)
					evidence["ai_decision"], evidence["ai_confidence"] = review.Decision, review.Confidence
					if err := s.recordCanonicalVerificationFailure(ctx, p.ID, fix, app, "ai_verification_inconclusive", reason, evidence, callID, acquired.Marker); err != nil {
						return err
					}
				} else if err := s.scheduleCanonicalVerificationPollWithMarker(ctx, q, p.ID, app, now, acquired.Marker); err != nil {
					return err
				}
				continue
			}
			if canonicalSiteFixVerifyExpired(app, now) {
				if err := s.recordCanonicalVerificationFailure(ctx, p.ID, fix, app, "manual_verification_required", "stored acceptance tests cannot be executed deterministically", canonicalPageEvidenceMap(targetURL, page, results), pgtype.UUID{}, carriedMarker); err != nil {
					return err
				}
			} else if err := s.scheduleCanonicalVerificationPollWithMarker(ctx, q, p.ID, app, now, carriedMarker); err != nil {
				return err
			}
			continue
		}
		if passed {
			if err := s.markCanonicalDeploymentObserved(ctx, q, p.ID, &fix, &app, targetURL, page, "deterministic_acceptance_match"); err != nil {
				return err
			}
			if err := s.recordCanonicalVerificationPass(ctx, p.ID, fix, app, canonicalPageEvidenceMap(targetURL, page, results), pgtype.UUID{}, carriedMarker); err != nil {
				return err
			}
			continue
		}
		if canonicalSiteFixVerifyExpired(app, now) {
			if err := s.recordCanonicalVerificationFailure(ctx, p.ID, fix, app, "acceptance_failed", "one or more stored acceptance tests failed", canonicalPageEvidenceMap(targetURL, page, results), pgtype.UUID{}, carriedMarker); err != nil {
				return err
			}
			continue
		}
		if err := s.scheduleCanonicalVerificationPollWithMarker(ctx, q, p.ID, app, now, carriedMarker); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) scheduleCanonicalVerificationPoll(ctx context.Context, q *db.Queries, projectID uuid.UUID, app db.SiteChangeApplication, now time.Time) error {
	return q.SetCanonicalSiteFixNextPollAt(ctx, db.SetCanonicalSiteFixNextPollAtParams{ApplicationID: app.ID, ProjectID: projectID, SiteFixID: app.SiteFixID, NextPollAt: pgutil.TS(now.Add(5 * time.Minute))})
}

func (s *Scheduler) scheduleCanonicalVerificationPollWithMarker(ctx context.Context, q *db.Queries, projectID uuid.UUID, app db.SiteChangeApplication, now time.Time, marker *db.DoctorAiOnDemandTrigger) error {
	if marker == nil {
		return s.scheduleCanonicalVerificationPoll(ctx, q, projectID, app, now)
	}
	return s.withCanonicalVerificationTx(ctx, func(txq *db.Queries) error {
		if err := s.scheduleCanonicalVerificationPoll(ctx, txq, projectID, app, now); err != nil {
			return err
		}
		return markCanonicalAIReviewAppliedStrict(ctx, txq, marker)
	})
}

func (s *Scheduler) markCanonicalDeploymentObserved(ctx context.Context, q *db.Queries, projectID uuid.UUID, fix *db.SiteFix, app *db.SiteChangeApplication, targetURL string, page canonicalPageEvidence, source string) error {
	if fix.Status != "awaiting_deploy" && fix.Status != "reopened" {
		return nil
	}
	now := s.currentTime()
	deployment := json.RawMessage(mustJSON(map[string]any{"source": source, "target_url": targetURL, "http_status": page.StatusCode, "response_headers": canonicalEvidenceHeaders(page.Headers), "observed_at": now.UTC().Format(time.RFC3339)}))
	if _, err := q.MarkCanonicalSiteFixVerifying(ctx, db.MarkCanonicalSiteFixVerifyingParams{SiteFixID: fix.ID, ProjectID: projectID, ApplicationID: app.ID, DeploymentSnapshot: deployment, DeployedAt: pgutil.TS(now)}); err != nil {
		return err
	}
	fix.Status, app.Status, app.DeploymentSnapshot = "verifying", "verification_pending", deployment
	return nil
}

func canonicalPageEvidenceMap(targetURL string, page canonicalPageEvidence, acceptance any) map[string]any {
	return map[string]any{"target_url": targetURL, "final_url": page.FinalURL, "redirect_chain": page.RedirectChain, "http_status": page.StatusCode, "response_headers": canonicalEvidenceHeaders(page.Headers), "acceptance_results": acceptance}
}

func canonicalSiteFixVerifyExpired(app db.SiteChangeApplication, now time.Time) bool {
	return now.Sub(siteFixMergedAt(app)) >= siteFixVerifyDeadline
}

func evaluateCanonicalAcceptanceTests(fix db.SiteFix, app db.SiteChangeApplication, page canonicalPageEvidence) ([]map[string]any, bool, bool) {
	var tests []json.RawMessage
	if json.Unmarshal(fix.AcceptanceTests, &tests) != nil || len(tests) == 0 {
		return nil, false, false
	}
	doc, err := xhtml.Parse(strings.NewReader(page.Body))
	if err != nil {
		return []map[string]any{{"status": "error", "reason": "html_parse_failed"}}, false, true
	}
	results := make([]map[string]any, 0, len(tests))
	allPassed := page.StatusCode == http.StatusOK
	allExecutable := true
	for i, raw := range tests {
		var test struct {
			Type         string `json:"type"`
			Expected     string `json:"expected"`
			ExpectedType string `json:"expected_type"`
		}
		result := map[string]any{"index": i}
		if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &test) != nil {
			result["status"], result["reason"] = "manual_required", "typed_acceptance_required"
			results = append(results, result)
			allExecutable, allPassed = false, false
			continue
		}
		test.Type = strings.ToLower(strings.TrimSpace(test.Type))
		result["type"] = test.Type
		passed, executable, observed := executeCanonicalDOMAcceptance(doc, page.Headers, test.Type, strings.TrimSpace(test.Expected), strings.TrimSpace(test.ExpectedType))
		result["observed"] = observed
		if !executable {
			result["status"], result["reason"] = "manual_required", "unsupported_acceptance_type"
			allExecutable, allPassed = false, false
		} else if passed {
			result["status"] = "passed"
		} else {
			result["status"] = "failed"
			allPassed = false
		}
		results = append(results, result)
	}
	return results, allPassed && allExecutable, allExecutable
}

func executeCanonicalDOMAcceptance(doc *xhtml.Node, headers http.Header, kind, expected, expectedType string) (bool, bool, any) {
	head := canonicalHead(doc)
	if head == nil {
		return false, true, map[string]any{"reason": "head_missing"}
	}
	switch kind {
	case "title_equals":
		if expected == "" {
			return false, false, nil
		}
		observed := canonicalElementText(head, "title")
		return observed == expected, true, observed
	case "meta_description_equals":
		if expected == "" {
			return false, false, nil
		}
		observed := canonicalMetaContents(head, "description")
		return len(observed) == 1 && observed[0] == expected, true, observed
	case "canonical_present":
		observed, valid := canonicalNormalizedLinks(head)
		return valid && len(observed) == 1, true, observed
	case "canonical_equals":
		if expected == "" {
			return false, false, nil
		}
		observed, valid := canonicalNormalizedLinks(head)
		normalizedExpected, expectedValid := normalizeCanonicalURL(expected)
		return valid && expectedValid && len(observed) == 1 && observed[0] == normalizedExpected, true, observed
	case "noindex_absent":
		metaValues := canonicalMetaContents(head, "robots")
		headerValues := append([]string(nil), headers.Values("X-Robots-Tag")...)
		observed := map[string]any{"meta_robots": metaValues, "x_robots_tag": headerValues}
		for _, value := range append(metaValues, headerValues...) {
			if containsDirective(value, "noindex") || containsDirective(value, "none") {
				return false, true, observed
			}
		}
		return true, true, observed
	case "json_ld_valid":
		valid, observed := canonicalJSONLDValid(head, expectedType)
		return valid, true, observed
	default:
		return false, false, nil
	}
}

func canonicalHead(doc *xhtml.Node) *xhtml.Node {
	if doc == nil {
		return nil
	}
	if doc.Type == xhtml.ElementNode && strings.EqualFold(doc.Data, "head") {
		return doc
	}
	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		if head := canonicalHead(child); head != nil {
			return head
		}
	}
	return nil
}

func canonicalElementText(doc *xhtml.Node, tag string) string {
	var walk func(*xhtml.Node) string
	walk = func(node *xhtml.Node) string {
		if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, tag) {
			var b strings.Builder
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == xhtml.TextNode {
					b.WriteString(child.Data)
				}
			}
			return strings.TrimSpace(b.String())
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if value := walk(child); value != "" {
				return value
			}
		}
		return ""
	}
	return walk(doc)
}

func canonicalMetaContents(doc *xhtml.Node, name string) []string {
	return canonicalAttributesBySelector(doc, "meta", "name", name, "content")
}

func canonicalAttributesBySelector(doc *xhtml.Node, tag, selectorAttr, selectorValue, resultAttr string) []string {
	var results []string
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, tag) {
			selector, result := "", ""
			for _, attr := range node.Attr {
				switch strings.ToLower(attr.Key) {
				case selectorAttr:
					selector = attr.Val
				case resultAttr:
					result = attr.Val
				}
			}
			for _, token := range strings.Fields(strings.ToLower(selector)) {
				if token == strings.ToLower(selectorValue) {
					results = append(results, strings.TrimSpace(result))
					break
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return results
}

func canonicalNormalizedLinks(head *xhtml.Node) ([]string, bool) {
	hrefs := canonicalAttributesBySelector(head, "link", "rel", "canonical", "href")
	normalized := make([]string, 0, len(hrefs))
	allValid := len(hrefs) > 0
	for _, href := range hrefs {
		value, valid := normalizeCanonicalURL(href)
		if !valid {
			allValid = false
			value = href
		}
		normalized = append(normalized, value)
	}
	return normalized, allValid
}

func normalizeCanonicalURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	parsed.Host = host
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), true
}

func containsDirective(content, directive string) bool {
	for _, value := range strings.FieldsFunc(strings.ToLower(content), func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if value == directive {
			return true
		}
	}
	return false
}

func canonicalJSONLDValid(doc *xhtml.Node, expectedType string) (bool, any) {
	type observation struct {
		Types   []string `json:"types"`
		Context bool     `json:"schema_context"`
		Valid   bool     `json:"valid"`
	}
	seen := []observation{}
	allValid := true
	matchedType := expectedType == ""
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, "script") {
			isJSONLD := false
			for _, attr := range node.Attr {
				if strings.EqualFold(attr.Key, "type") && strings.EqualFold(strings.TrimSpace(attr.Val), "application/ld+json") {
					isJSONLD = true
				}
			}
			if isJSONLD {
				var raw strings.Builder
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == xhtml.TextNode {
						raw.WriteString(child.Data)
					}
				}
				var value any
				validJSON := json.Unmarshal([]byte(raw.String()), &value) == nil
				types := collectJSONLDTypes(value)
				hasContext := validJSON && hasSchemaJSONLDContext(value)
				valid := validJSON && hasContext && len(types) > 0 && !jsonLDContainsUnsafeMarker(value)
				seen = append(seen, observation{Types: types, Context: hasContext, Valid: valid})
				allValid = allValid && valid
				matchedType = matchedType || containsFold(types, expectedType)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return len(seen) > 0 && allValid && matchedType, seen
}

func jsonLDContainsUnsafeMarker(value any) bool {
	switch typed := value.(type) {
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		for _, marker := range []string{"{{", "}}", "${", "<%", "%>", "placeholder", "replace_me", "todo", "tbd", "lorem ipsum"} {
			if strings.Contains(normalized, marker) {
				return true
			}
		}
	case map[string]any:
		for key, child := range typed {
			if jsonLDContainsUnsafeMarker(key) || jsonLDContainsUnsafeMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonLDContainsUnsafeMarker(child) {
				return true
			}
		}
	}
	return false
}

func hasSchemaJSONLDContext(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if contextValue, ok := typed["@context"]; ok && jsonLDContextContainsSchema(contextValue) {
			return true
		}
		for key, child := range typed {
			if key != "@context" && hasSchemaJSONLDContext(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasSchemaJSONLDContext(child) {
				return true
			}
		}
	}
	return false
}

func jsonLDContextContainsSchema(value any) bool {
	switch typed := value.(type) {
	case string:
		normalized := strings.TrimRight(strings.ToLower(strings.TrimSpace(typed)), "/")
		return normalized == "https://schema.org" || normalized == "http://schema.org"
	case []any:
		for _, child := range typed {
			if jsonLDContextContainsSchema(child) {
				return true
			}
		}
	}
	return false
}

func collectJSONLDTypes(value any) []string {
	var out []string
	switch typed := value.(type) {
	case map[string]any:
		if value, ok := typed["@type"].(string); ok {
			out = append(out, value)
		}
		for _, child := range typed {
			out = append(out, collectJSONLDTypes(child)...)
		}
	case []any:
		for _, child := range typed {
			out = append(out, collectJSONLDTypes(child)...)
		}
	}
	return out
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}

type canonicalPageEvidence struct {
	Body          string
	StatusCode    int
	Headers       http.Header
	FinalURL      string
	RedirectChain []string
}

type canonicalSiteFixPageVerifier interface {
	Fetch(context.Context, string, string) (canonicalPageEvidence, error)
}

type safeCanonicalPageVerifier struct {
	resolver *net.Resolver
}

func (s *Scheduler) fetchCanonicalSiteFixPage(ctx context.Context, projectSiteURL, targetURL string) (canonicalPageEvidence, error) {
	verifier := s.siteFixVerifier
	if verifier == nil {
		verifier = safeCanonicalPageVerifier{}
	}
	return verifier.Fetch(ctx, projectSiteURL, targetURL)
}

func (v safeCanonicalPageVerifier) Fetch(ctx context.Context, projectSiteURL, targetURL string) (canonicalPageEvidence, error) {
	project, err := url.Parse(strings.TrimSpace(projectSiteURL))
	if err != nil {
		return canonicalPageEvidence{}, fmt.Errorf("parse project site URL: %w", err)
	}
	target, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return canonicalPageEvidence{}, fmt.Errorf("parse canonical verification URL: %w", err)
	}
	if err := validateCanonicalVerificationURL(project, target); err != nil {
		return canonicalPageEvidence{}, err
	}
	resolver := v.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolver.LookupIPAddr(dialCtx, host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("resolve verification target: %w", err)
			}
			for _, resolved := range ips {
				if !safeCanonicalVerificationIP(resolved.IP) {
					return nil, fmt.Errorf("verification target resolved to a prohibited address")
				}
			}
			return dialer.DialContext(dialCtx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	redirectChain := []string{target.String()}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many verification redirects")
			}
			if err := validateCanonicalVerificationURL(project, req.URL); err != nil {
				return err
			}
			redirectChain = append(redirectChain, req.URL.String())
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return canonicalPageEvidence{}, err
	}
	req.Header.Set("User-Agent", "CiteLoopVerifier/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return canonicalPageEvidence{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, siteFixVerifyMaxBody+1))
	if err != nil {
		return canonicalPageEvidence{StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), FinalURL: resp.Request.URL.String(), RedirectChain: redirectChain}, err
	}
	if len(body) > siteFixVerifyMaxBody {
		return canonicalPageEvidence{StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), FinalURL: resp.Request.URL.String(), RedirectChain: redirectChain}, errors.New("verification response exceeded body limit")
	}
	evidence := canonicalPageEvidence{Body: string(body), StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), FinalURL: resp.Request.URL.String(), RedirectChain: redirectChain}
	if resp.StatusCode >= 400 {
		return evidence, &siteFixHTTPError{status: resp.StatusCode}
	}
	return evidence, nil
}

func canonicalEvidenceHeaders(headers http.Header) map[string][]string {
	result := make(map[string][]string)
	for name, values := range headers {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "set-cookie", "cookie", "authorization", "proxy-authorization":
			continue
		}
		result[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
	}
	return result
}

func validateCanonicalVerificationURL(project, target *url.URL) error {
	if project == nil || target == nil || target.User != nil || target.Hostname() == "" {
		return errors.New("invalid canonical verification URL")
	}
	if (target.Scheme != "http" && target.Scheme != "https") || project.Scheme != target.Scheme {
		return errors.New("canonical verification URL must use the configured HTTP(S) scheme")
	}
	if !strings.EqualFold(project.Hostname(), target.Hostname()) || effectiveURLPort(project) != effectiveURLPort(target) {
		return errors.New("canonical verification URL must be same-origin with the project site")
	}
	if ip := net.ParseIP(target.Hostname()); ip != nil && !safeCanonicalVerificationIP(ip) {
		return errors.New("canonical verification URL uses a prohibited address")
	}
	return nil
}

func effectiveURLPort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if value.Scheme == "https" {
		return "443"
	}
	return "80"
}

var prohibitedVerificationCIDRs = func() []*net.IPNet {
	values := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32",
	}
	out := make([]*net.IPNet, 0, len(values))
	for _, raw := range values {
		_, block, _ := net.ParseCIDR(raw)
		out = append(out, block)
	}
	return out
}()

func safeCanonicalVerificationIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, block := range prohibitedVerificationCIDRs {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func (s *Scheduler) recordCanonicalVerificationPass(ctx context.Context, projectID uuid.UUID, fix db.SiteFix, app db.SiteChangeApplication, evidence map[string]any, aiCallID pgtype.UUID, marker *db.DoctorAiOnDemandTrigger) error {
	return s.withCanonicalVerificationTx(ctx, func(q *db.Queries) error {
		now := s.currentTime()
		snapshot := json.RawMessage(mustJSON(map[string]any{"result": "passed", "evidence": evidence, "verified_at": now.UTC().Format(time.RFC3339)}))
		if _, err := q.AppendCanonicalSiteFixVerification(ctx, db.AppendCanonicalSiteFixVerificationParams{
			ID: uuid.New(), ProjectID: projectID, SiteFixID: fix.ID, AttemptNumber: fix.RetryCount + 1,
			EvidenceRead: json.RawMessage(mustJSON(evidence)), AcceptanceResults: json.RawMessage(mustJSON(evidence["acceptance_results"])),
			AiCallID: aiCallID, Result: "passed", RetryClassification: canonicalRetryClassification(true, fix.RetryCount, fix.MaxRetries), AttemptedAt: pgutil.TS(now),
		}); err != nil {
			return err
		}
		if err := markCanonicalAIReviewAppliedStrict(ctx, q, marker); err != nil {
			return err
		}
		if err := supersedeCanonicalAISiblingMarkers(ctx, q, projectID, fix.ID, marker); err != nil {
			return err
		}
		_, err := q.MarkCanonicalSiteFixVerified(ctx, db.MarkCanonicalSiteFixVerifiedParams{
			SiteFixID: fix.ID, ProjectID: projectID, ApplicationID: app.ID,
			DeploymentSnapshot: rawJSONOrObject(app.DeploymentSnapshot), VerificationSnapshot: snapshot, VerifiedAt: pgutil.TS(now),
		})
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *Scheduler) recordCanonicalVerificationFailure(ctx context.Context, projectID uuid.UUID, fix db.SiteFix, app db.SiteChangeApplication, reasonCode, reason string, evidence map[string]any, aiCallID pgtype.UUID, marker *db.DoctorAiOnDemandTrigger) error {
	return s.withCanonicalVerificationTx(ctx, func(q *db.Queries) error {
		now := s.currentTime()
		snapshot := json.RawMessage(mustJSON(map[string]any{"result": "failed", "reason_code": reasonCode, "reason": reason, "evidence": evidence, "attempted_at": now.UTC().Format(time.RFC3339)}))
		retryClass := canonicalRetryClassification(false, fix.RetryCount, fix.MaxRetries)
		if _, err := q.AppendCanonicalSiteFixVerification(ctx, db.AppendCanonicalSiteFixVerificationParams{
			ID: uuid.New(), ProjectID: projectID, SiteFixID: fix.ID, AttemptNumber: fix.RetryCount + 1,
			EvidenceRead: json.RawMessage(mustJSON(evidence)), AcceptanceResults: json.RawMessage(mustJSON(evidence["acceptance_results"])),
			AiCallID: aiCallID, Result: "failed", RetryClassification: retryClass, FailureReason: &reason, AttemptedAt: pgutil.TS(now),
		}); err != nil {
			return err
		}
		if err := markCanonicalAIReviewAppliedStrict(ctx, q, marker); err != nil {
			return err
		}
		if err := supersedeCanonicalAISiblingMarkers(ctx, q, projectID, fix.ID, marker); err != nil {
			return err
		}
		if retryClass == "retryable" {
			if _, err := q.MarkCanonicalSiteFixRetryable(ctx, db.MarkCanonicalSiteFixRetryableParams{SiteFixID: fix.ID, ProjectID: projectID, ApplicationID: app.ID, VerificationSnapshot: snapshot, FailureReason: &reason}); err != nil {
				return err
			}
			return nil
		}
		if _, err := q.TerminalizeCanonicalSiteFix(ctx, db.TerminalizeCanonicalSiteFixParams{SiteFixID: fix.ID, ProjectID: projectID, ApplicationID: app.ID, VerificationSnapshot: snapshot, FailureReason: &reason, ForceTerminal: retryClass == "retry_exhausted"}); err != nil {
			return err
		}
		return nil
	})
}

func canonicalRetryClassification(passed bool, retryCount, maxRetries int32) string {
	if passed {
		return "not_applicable"
	}
	if retryCount >= maxRetries {
		return "retry_exhausted"
	}
	return "retryable"
}

func rawJSONOrObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (s *Scheduler) withCanonicalVerificationTx(ctx context.Context, fn func(*db.Queries) error) error {
	if s.Pool == nil {
		return errors.New("canonical Site Fix verification database unavailable")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	s = htmlstd.UnescapeString(s)
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
	// Advance the content action into the measurement loop (same transition the
	// manual "Mark applied" path uses), so the fix enters attribution. The
	// application and action move together in one SQL statement so a failed
	// action update cannot strand the application as verified.
	if _, err := q.MarkSiteChangeApplicationAndContentActionVerified(ctx, db.MarkSiteChangeApplicationAndContentActionVerifiedParams{
		ApplicationID:        app.ID,
		ProjectID:            p.ID,
		DeploymentSnapshot:   json.RawMessage(`{}`),
		VerifiedAt:           pgutil.TS(now),
		VerificationSnapshot: snapRaw,
		PublisherResult:      siteFixVerifiedPublisherResult(app, source, now),
	}); err != nil {
		return err
	}
	s.Log.Info("site fix verified", "project", p.ID, "application", app.ID, "source", source, "target_url", app.TargetUrl)
	return nil
}

func siteFixVerifiedPublisherResult(app db.SiteChangeApplication, source string, verifiedAt time.Time) json.RawMessage {
	return json.RawMessage(mustJSON(map[string]any{
		"mode":                       "github_pr",
		"status":                     "verified",
		"site_change_application_id": app.ID,
		"github_pr_number":           app.GithubPrNumber,
		"github_pr_url":              app.GithubPrUrl,
		"github_pr_state":            "merged",
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"target_url":                 app.TargetUrl,
		"verification_source":        source,
		"verified_at":                verifiedAt.UTC().Format(time.RFC3339),
	}))
}
