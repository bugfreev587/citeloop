package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
)

type RepositoryTarget struct {
	Repo          string
	Branch        string
	BaseCommitSHA string
}

const (
	MaxRepositorySourceCandidates       = 128
	MaxRepositoryCandidateMetadataBytes = 24 * 1024
)

type RepositorySourceCandidate struct {
	Path  string `json:"path"`
	SHA   string `json:"-"`
	Size  int64  `json:"size"`
	Score int    `json:"-"`
}

type RepositorySourceLoader interface {
	Candidates(context.Context, db.SiteFix) (RepositoryTarget, []RepositorySourceCandidate, error)
	LoadSelected(context.Context, RepositoryTarget, []string) (RepositorySnapshot, error)
}

type RepositorySourceSelector interface {
	Describe(db.SiteFix, []RepositorySourceCandidate) GenerationCall
	Select(context.Context, db.SiteFix, []RepositorySourceCandidate, siteFixAICallAttempt) ([]string, GenerationResult, error)
}

// RankRepositorySourceCandidates filters unsafe/non-source paths before any
// model call, then applies deterministic hints based on the finding and target.
func RankRepositorySourceCandidates(fix db.SiteFix, candidates []RepositorySourceCandidate, treeTruncated bool) ([]RepositorySourceCandidate, error) {
	if treeTruncated {
		return nil, errors.New("repository tree is truncated")
	}
	contextText := strings.ToLower(strings.Join([]string{fix.FindingKind, string(fix.ProposedFix), string(fix.AcceptanceTests), string(fix.EvidenceSnapshot)}, " "))
	targetTokens := repositoryTargetTokens(fix.TargetUrls)
	out := make([]RepositorySourceCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if !safeRepositoryCandidate(candidate) {
			continue
		}
		if _, duplicate := seen[candidate.Path]; duplicate {
			continue
		}
		seen[candidate.Path] = struct{}{}
		candidate.Score = repositoryCandidateScore(contextText, targetTokens, candidate.Path)
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return nil, errors.New("repository tree contains no bounded safe source candidates")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	out = boundedRepositorySourceCandidates(out)
	if len(out) == 0 {
		return nil, errors.New("repository tree contains no bounded safe source candidate metadata")
	}
	return out, nil
}

type LLMRepositorySourceSelector struct {
	Provider llm.Provider
	Model    string
}

func (s LLMRepositorySourceSelector) Describe(fix db.SiteFix, candidates []RepositorySourceCandidate) GenerationCall {
	req := s.completionRequest(fix, candidates)
	return GenerationCall{
		Provider: "tokengate", Model: firstNonEmpty(s.Model, llm.DefaultTokenGateModel),
		PromptVersion: "doctor-repository-source-selection-v1", RequestFingerprint: aicalls.Fingerprint(req),
	}
}

func (s LLMRepositorySourceSelector) completionRequest(fix db.SiteFix, candidates []RepositorySourceCandidate) llm.CompletionReq {
	metadata := make([]repositoryCandidateMetadata, 0, min(len(candidates), MaxRepositorySourceCandidates))
	for _, candidate := range boundedRepositorySourceCandidates(candidates) {
		metadata = append(metadata, repositoryCandidateMetadata{Path: candidate.Path, Size: candidate.Size})
	}
	prompt, _ := json.Marshal(map[string]any{
		"finding_kind": fix.FindingKind, "target_urls": json.RawMessage(fix.TargetUrls),
		"proposed_fix": json.RawMessage(fix.ProposedFix), "acceptance_tests": json.RawMessage(fix.AcceptanceTests),
		"candidates": metadata,
	})
	return llm.CompletionReq{
		System:  "Select the smallest set of existing repository source files likely to implement this approved Site Fix. Use only candidate path and size metadata. Never invent paths. Return strict JSON only.",
		Prompt:  "Return exactly {\"paths\":[string]}. Select at most eight paths and only from candidates.\n" + string(prompt),
		Purpose: llm.PurposeSiteFix, Model: firstNonEmpty(s.Model, llm.DefaultTokenGateModel), JSON: true,
		MaxTokens: 500, DisableProviderFallback: true,
	}
}

func (s LLMRepositorySourceSelector) Select(ctx context.Context, fix db.SiteFix, candidates []RepositorySourceCandidate, attempt siteFixAICallAttempt) ([]string, GenerationResult, error) {
	if s.Provider == nil {
		return nil, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "provider_unavailable"}, errors.New("repository source selector provider is unavailable")
	}
	boundedCandidates := boundedRepositorySourceCandidates(candidates)
	safe := make(map[string]RepositorySourceCandidate, len(boundedCandidates))
	for _, candidate := range boundedCandidates {
		safe[candidate.Path] = candidate
	}
	if len(safe) == 0 {
		return nil, GenerationResult{Status: "skipped", ErrorCode: "no_safe_candidates"}, errors.New("repository source selector has no safe candidates")
	}
	req := s.completionRequest(fix, candidates)
	req.AttemptObserver = attempt
	resp, err := llm.CompleteObserved(ctx, s.Provider, req)
	result := GenerationResult{
		Provider: firstNonEmpty(resp.Provider, "tokengate"), Model: firstNonEmpty(resp.Model, s.Model), Status: "ok",
		PromptTokens: int32(max(resp.PromptTokens, 0)), CompletionTokens: int32(max(resp.CompletionTokens, 0)),
		TotalTokens: int32(max(resp.Tokens, 0)), CostUSD: resp.CostUSD,
	}
	if err != nil {
		result.Status, result.ErrorCode = "failed", "provider_error"
		return nil, result, err
	}
	var output struct {
		Paths []string `json:"paths"`
	}
	if err := decodeJSONObject(resp.Text, &output); err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_response"
		return nil, result, err
	}
	paths := make([]string, 0, min(len(output.Paths), MaxRepositorySourceFiles))
	seen := make(map[string]struct{}, len(output.Paths))
	var total int64
	for _, selected := range output.Paths {
		candidate, ok := safe[selected]
		if !ok {
			continue
		}
		if _, duplicate := seen[selected]; duplicate {
			continue
		}
		if len(paths) == MaxRepositorySourceFiles {
			result.Status, result.ErrorCode = "failed", "selection_over_limit"
			return nil, result, fmt.Errorf("repository source selection exceeds %d files", MaxRepositorySourceFiles)
		}
		total += candidate.Size
		if total > MaxRepositorySourceTotalBytes {
			result.Status, result.ErrorCode = "failed", "selection_over_limit"
			return nil, result, fmt.Errorf("repository source selection exceeds %d bytes", MaxRepositorySourceTotalBytes)
		}
		seen[selected] = struct{}{}
		paths = append(paths, selected)
	}
	if len(paths) == 0 {
		result.Status, result.ErrorCode = "failed", "empty_safe_selection"
		return nil, result, errors.New("repository source selector did not choose a safe existing path")
	}
	return paths, result, nil
}

type repositoryCandidateMetadata struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func boundedRepositorySourceCandidates(candidates []RepositorySourceCandidate) []RepositorySourceCandidate {
	out := make([]RepositorySourceCandidate, 0, min(len(candidates), MaxRepositorySourceCandidates))
	seen := make(map[string]struct{}, min(len(candidates), MaxRepositorySourceCandidates))
	metadataBytes := 2 // JSON array brackets.
	for _, candidate := range candidates {
		if len(out) == MaxRepositorySourceCandidates || !safeRepositoryCandidate(candidate) {
			if len(out) == MaxRepositorySourceCandidates {
				break
			}
			continue
		}
		if _, duplicate := seen[candidate.Path]; duplicate {
			continue
		}
		encoded, err := json.Marshal(repositoryCandidateMetadata{Path: candidate.Path, Size: candidate.Size})
		if err != nil {
			continue
		}
		nextBytes := metadataBytes + len(encoded)
		if len(out) > 0 {
			nextBytes++ // comma
		}
		if nextBytes > MaxRepositoryCandidateMetadataBytes {
			break
		}
		metadataBytes = nextBytes
		seen[candidate.Path] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func safeRepositoryCandidate(candidate RepositorySourceCandidate) bool {
	if _, err := validateRepositoryPath(candidate.Path); err != nil || strings.TrimSpace(candidate.SHA) == "" || candidate.Size < 0 || candidate.Size > MaxRepositorySourceFileBytes {
		return false
	}
	lower := strings.ToLower(candidate.Path)
	parts := strings.Split(lower, "/")
	blockedDirs := map[string]struct{}{
		".git": {}, ".github": {}, ".next": {}, ".nuxt": {}, "node_modules": {}, "vendor": {},
		"dist": {}, "build": {}, "out": {}, "coverage": {}, "target": {}, "generated": {}, "__generated__": {},
	}
	for _, part := range parts[:len(parts)-1] {
		if _, blocked := blockedDirs[part]; blocked {
			return false
		}
	}
	base := parts[len(parts)-1]
	if base == "jenkinsfile" || base == ".gitlab-ci.yml" || strings.HasPrefix(base, ".env") ||
		strings.Contains(base, "secret") || strings.Contains(base, "credential") ||
		strings.HasSuffix(base, ".lock") || strings.HasSuffix(base, "-lock.json") ||
		base == "go.sum" || base == "composer.lock" || base == "gemfile.lock" || base == "cargo.lock" {
		return false
	}
	ext := strings.ToLower(path.Ext(base))
	allowed := map[string]struct{}{
		".astro": {}, ".css": {}, ".go": {}, ".hbs": {}, ".htm": {}, ".html": {}, ".js": {}, ".json": {}, ".jsonld": {},
		".jsx": {}, ".liquid": {}, ".md": {}, ".mdx": {}, ".njk": {}, ".php": {}, ".py": {}, ".rb": {}, ".scss": {},
		".svelte": {}, ".toml": {}, ".ts": {}, ".tsx": {}, ".txt": {}, ".twig": {}, ".vue": {}, ".xml": {}, ".yaml": {}, ".yml": {},
	}
	_, ok := allowed[ext]
	return ok
}

func repositoryTargetTokens(raw json.RawMessage) []string {
	var targets []string
	_ = json.Unmarshal(raw, &targets)
	seen := map[string]struct{}{}
	var out []string
	for _, target := range targets {
		parsed, err := url.Parse(strings.TrimSpace(target))
		if err != nil {
			continue
		}
		for _, token := range strings.FieldsFunc(strings.ToLower(parsed.Path), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len(token) < 3 {
				continue
			}
			if _, duplicate := seen[token]; !duplicate {
				seen[token] = struct{}{}
				out = append(out, token)
			}
		}
	}
	return out
}

func repositoryCandidateScore(contextText string, targetTokens []string, candidatePath string) int {
	lower := strings.ToLower(candidatePath)
	base := strings.TrimSuffix(path.Base(lower), path.Ext(lower))
	score := 10
	for _, token := range targetTokens {
		if strings.Contains(lower, token) {
			score += 30
		}
	}
	families := []struct {
		triggers []string
		hints    []string
	}{
		{[]string{"sitemap"}, []string{"sitemap"}},
		{[]string{"canonical"}, []string{"canonical", "layout", "head", "seo"}},
		{[]string{"schema", "structured data", "json-ld", "jsonld"}, []string{"schema", "structured", "jsonld", "json-ld"}},
		{[]string{"internal-link", "internal link"}, []string{"navigation", "nav", "menu", "sidebar", "link"}},
		{[]string{"robots"}, []string{"robots"}},
		{[]string{"metadata", "title", "description"}, []string{"page", "metadata", "head", "seo"}},
	}
	for _, family := range families {
		if !containsAny(contextText, family.triggers) {
			continue
		}
		for _, hint := range family.hints {
			if strings.Contains(lower, hint) {
				score += 70
			}
		}
	}
	if base == "page" || base == "layout" || base == "index" || base == "robots" || base == "sitemap" {
		score += 5
	}
	return score
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
