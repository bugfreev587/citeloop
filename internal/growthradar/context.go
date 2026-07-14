package growthradar

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

type ClassifiedTerm struct {
	Value    string `json:"value"`
	Class    string `json:"class"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
}

type EvidenceIndex struct {
	PublicTerms          []string `json:"public_terms"`
	SuggestedTerms       []string `json:"suggested_terms"`
	SuggestedCompetitors []string `json:"suggested_competitors"`
}

type Classification struct {
	Terms                []ClassifiedTerm `json:"terms"`
	AcceptedVocabulary   []string         `json:"accepted_vocabulary"`
	ConfirmedCompetitors []string         `json:"confirmed_competitors"`
}

// Public technical subjects are valid discovery topics. Block disclosure-shaped
// values and explicitly private implementation context, not nouns such as
// "Postgres", "API key", or "encryption" in an educational title.
var internalTermPattern = regexp.MustCompile(`(?i)(-----BEGIN(?: [A-Z]+)? PRIVATE KEY-----|(?:api[_ -]?key|access[_ -]?token|secret|password|credential)[A-Z0-9_ -]*[=:][[:space:]]*[^[:space:]]{8,}|(?:postgres(?:ql)?|mysql|redis)://[^[:space:]@]+:[^[:space:]@]+@|(?:sk|gh[opsu])[-_][A-Z0-9-]{16,}|internal[ _-]?(?:diagnostic|endpoint|hostname|runbook)|private[ _-](?:repo|repository|network|endpoint)|(?:localhost|127\.0\.0\.1)(?::[0-9]+)?)`)

func ContainsInternalSensitiveTerm(value string) bool { return internalTermPattern.MatchString(value) }

func ClassifyContext(profile json.RawMessage, evidence EvidenceIndex) Classification {
	var raw map[string]any
	_ = json.Unmarshal(profile, &raw)
	result := Classification{Terms: []ClassifiedTerm{}, AcceptedVocabulary: []string{}, ConfirmedCompetitors: []string{}}
	seen := map[string]struct{}{}
	add := func(value, class, reason string, accepted bool) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		if internalTermPattern.MatchString(value) {
			class, reason, accepted = "internal_sensitive", "matched internal or sensitive implementation pattern", false
		}
		result.Terms = append(result.Terms, ClassifiedTerm{Value: value, Class: class, Accepted: accepted, Reason: reason})
		if accepted {
			result.AcceptedVocabulary = append(result.AcceptedVocabulary, value)
		}
	}
	add(stringField(raw, "positioning"), "public_capability", "explicit project context", true)
	for _, value := range stringFields(raw, "value_props") {
		add(value, "public_capability", "explicit project context", true)
	}
	for _, value := range stringFields(raw, "features") {
		add(value, "public_capability", "explicit project context", true)
	}
	for _, value := range stringFields(raw, "icp") {
		add(value, "audience", "explicit project context", true)
	}
	for _, value := range stringFields(raw, "key_terms") {
		add(value, "search_language", "explicit project context", true)
	}
	for _, value := range stringFields(raw, "competitors") {
		add(value, "confirmed_competitor", "explicit project competitor", true)
		if !internalTermPattern.MatchString(value) {
			result.ConfirmedCompetitors = append(result.ConfirmedCompetitors, strings.TrimSpace(value))
		}
	}
	for _, value := range evidence.PublicTerms {
		add(value, "public_evidence", "qualifying public evidence", true)
	}
	for _, value := range evidence.SuggestedTerms {
		add(value, "unknown", "model suggestion lacks qualifying evidence", false)
	}
	for _, value := range evidence.SuggestedCompetitors {
		add(value, "unknown", "competitor is not configured in project context", false)
	}
	sort.Strings(result.AcceptedVocabulary)
	sort.Strings(result.ConfirmedCompetitors)
	return result
}

func DiscoveryProfile(profile json.RawMessage, evidence EvidenceIndex) json.RawMessage {
	classification := ClassifyContext(profile, evidence)
	encoded, _ := json.Marshal(map[string]any{
		"accepted_public_vocabulary": classification.AcceptedVocabulary,
		"confirmed_competitors":      classification.ConfirmedCompetitors,
		"classification_version":     "growth-radar-context-v1",
	})
	return encoded
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return strings.TrimSpace(value)
}

func stringFields(raw map[string]any, key string) []string {
	values, _ := raw[key].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}
