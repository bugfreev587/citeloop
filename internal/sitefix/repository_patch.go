package sitefix

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxRepositorySourceFiles             = 8
	MaxRepositorySourceFileBytes         = 128 * 1024
	MaxRepositorySourceTotalBytes        = 512 * 1024
	MaxRepositoryReplacementsPerFile     = 64
	MaxRepositoryReplacementsTotal       = 128
	MaxRepositoryReplacementBytesPerFile = 256 * 1024
	MaxRepositoryReplacementBytesTotal   = 512 * 1024
)

type RepositorySource struct {
	Path    string `json:"path"`
	SHA     string `json:"sha"`
	Content string `json:"content"`
}

type RepositorySnapshot struct {
	Repo          string             `json:"repo"`
	Branch        string             `json:"branch"`
	BaseCommitSHA string             `json:"base_commit_sha"`
	Sources       []RepositorySource `json:"sources"`
}

type ExactReplacement struct {
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type RepositoryFilePatch struct {
	Path         string             `json:"path"`
	BaseSHA      string             `json:"base_sha"`
	Replacements []ExactReplacement `json:"replacements"`
}

type RepositoryPatch struct {
	Files []RepositoryFilePatch `json:"files"`
}

type RepositoryFileUpdate struct {
	Path    string
	BaseSHA string
	Content []byte
}

type repositoryActualDiff struct {
	Files []repositoryActualFileDiff `json:"files"`
}

type repositoryActualFileDiff struct {
	Path         string                   `json:"path"`
	BaseSHA      string                   `json:"base_sha"`
	BeforeSHA256 string                   `json:"before_sha256"`
	AfterSHA256  string                   `json:"after_sha256"`
	Changes      []repositoryActualChange `json:"changes"`
}

type repositoryActualChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// RepositoryPreparedPatch is the durable, source-pinned artifact created
// before any GitHub mutation. Content is intentionally excluded; exact
// replacements, immutable blob identities, and content hashes are retained.
type RepositoryPreparedPatch struct {
	Repo                  string                        `json:"repo"`
	BaseBranch            string                        `json:"base_branch"`
	BaseCommitSHA         string                        `json:"base_commit_sha"`
	Files                 []RepositoryPreparedFilePatch `json:"files"`
	SourceAggregateSHA256 string                        `json:"source_aggregate_sha256"`
	ResultAggregateSHA256 string                        `json:"result_aggregate_sha256"`
}

type RepositoryPreparedFilePatch struct {
	Path                string             `json:"path"`
	BaseSHA             string             `json:"base_sha"`
	SourceContentSHA256 string             `json:"source_content_sha256"`
	ResultContentSHA256 string             `json:"result_content_sha256"`
	Replacements        []ExactReplacement `json:"replacements"`
}

type replacementRange struct {
	start       int
	end         int
	replacement ExactReplacement
}

// ApplyRepositoryPatch validates a model-produced patch against an immutable
// repository snapshot and applies it entirely in memory. The returned diff is
// computed from the actual replacements applied here, never from model output.
func ApplyRepositoryPatch(snapshot RepositorySnapshot, patch RepositoryPatch) ([]RepositoryFileUpdate, json.RawMessage, error) {
	if err := ValidateRepositorySnapshot(snapshot); err != nil {
		return nil, nil, err
	}
	if len(patch.Files) == 0 || len(patch.Files) > MaxRepositorySourceFiles {
		return nil, nil, fmt.Errorf("repository patch must update between one and %d files", MaxRepositorySourceFiles)
	}
	sources := make(map[string]RepositorySource, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		sources[source.Path] = source
	}
	seen := make(map[string]struct{}, len(patch.Files))
	updates := make([]RepositoryFileUpdate, 0, len(patch.Files))
	actual := repositoryActualDiff{Files: make([]repositoryActualFileDiff, 0, len(patch.Files))}
	resultTotal := 0
	replacementTotal, replacementBytesTotal := 0, 0
	for _, filePatch := range patch.Files {
		path, err := validateRepositoryPath(filePatch.Path)
		if err != nil {
			return nil, nil, err
		}
		if _, duplicate := seen[path]; duplicate {
			return nil, nil, fmt.Errorf("repository patch contains duplicate path %q", path)
		}
		seen[path] = struct{}{}
		source, ok := sources[path]
		if !ok {
			return nil, nil, fmt.Errorf("repository patch path %q is not in the selected source snapshot", path)
		}
		baseSHA := strings.TrimSpace(filePatch.BaseSHA)
		if baseSHA == "" || baseSHA != source.SHA {
			return nil, nil, fmt.Errorf("repository patch base sha does not match selected source %q", path)
		}
		if len(filePatch.Replacements) == 0 || len(filePatch.Replacements) > MaxRepositoryReplacementsPerFile {
			return nil, nil, fmt.Errorf("repository patch for %q must contain between one and %d replacements", path, MaxRepositoryReplacementsPerFile)
		}
		replacementTotal += len(filePatch.Replacements)
		if replacementTotal > MaxRepositoryReplacementsTotal {
			return nil, nil, fmt.Errorf("repository patch exceeds %d total replacements", MaxRepositoryReplacementsTotal)
		}
		replacementBytes := 0
		for _, replacement := range filePatch.Replacements {
			if replacement.OldText == "" {
				return nil, nil, fmt.Errorf("repository patch for %q contains empty old_text", path)
			}
			if replacement.OldText == replacement.NewText {
				return nil, nil, fmt.Errorf("repository patch for %q contains an unchanged replacement", path)
			}
			if err := validateRepositoryText(replacement.OldText, "old_text"); err != nil {
				return nil, nil, fmt.Errorf("repository patch for %q: %w", path, err)
			}
			if err := validateRepositoryText(replacement.NewText, "new_text"); err != nil {
				return nil, nil, fmt.Errorf("repository patch for %q: %w", path, err)
			}
			if len(replacement.OldText) > MaxRepositoryReplacementBytesPerFile-replacementBytes {
				return nil, nil, fmt.Errorf("repository patch replacement text for %q exceeds %d bytes", path, MaxRepositoryReplacementBytesPerFile)
			}
			replacementBytes += len(replacement.OldText)
			if len(replacement.NewText) > MaxRepositoryReplacementBytesPerFile-replacementBytes {
				return nil, nil, fmt.Errorf("repository patch replacement text for %q exceeds %d bytes", path, MaxRepositoryReplacementBytesPerFile)
			}
			replacementBytes += len(replacement.NewText)
		}
		if replacementBytes > MaxRepositoryReplacementBytesTotal-replacementBytesTotal {
			return nil, nil, fmt.Errorf("repository patch exceeds %d total replacement bytes", MaxRepositoryReplacementBytesTotal)
		}
		replacementBytesTotal += replacementBytes
		original := []byte(source.Content)
		ranges := make([]replacementRange, 0, len(filePatch.Replacements))
		finalSize := len(original)
		for _, replacement := range filePatch.Replacements {
			old := []byte(replacement.OldText)
			if bytes.Count(original, old) != 1 {
				return nil, nil, fmt.Errorf("repository patch old_text must occur exactly once in %q", path)
			}
			start := bytes.Index(original, old)
			ranges = append(ranges, replacementRange{start: start, end: start + len(old), replacement: replacement})
			finalSize += len(replacement.NewText) - len(old)
		}
		sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
		for i := 1; i < len(ranges); i++ {
			if ranges[i].start < ranges[i-1].end {
				return nil, nil, fmt.Errorf("repository patch replacements overlap in %q", path)
			}
		}
		if finalSize <= 0 || finalSize > MaxRepositorySourceFileBytes || finalSize > MaxRepositorySourceTotalBytes-resultTotal {
			return nil, nil, fmt.Errorf("repository patch result for %q exceeds bounded output size", path)
		}
		result := make([]byte, 0, finalSize)
		cursor := 0
		for _, r := range ranges {
			result = append(result, original[cursor:r.start]...)
			result = append(result, r.replacement.NewText...)
			cursor = r.end
		}
		result = append(result, original[cursor:]...)
		if len(result) != finalSize || bytes.Equal(result, original) {
			return nil, nil, fmt.Errorf("repository patch for %q produced empty or unchanged content", path)
		}
		if len(result) > MaxRepositorySourceFileBytes || !utf8.Valid(result) || bytes.IndexByte(result, 0) >= 0 {
			return nil, nil, fmt.Errorf("repository patch result for %q is not bounded UTF-8 text", path)
		}
		resultTotal += finalSize
		changes := make([]repositoryActualChange, 0, len(ranges))
		for _, r := range ranges {
			changes = append(changes, repositoryActualChange{Before: r.replacement.OldText, After: r.replacement.NewText})
		}
		updates = append(updates, RepositoryFileUpdate{Path: path, BaseSHA: source.SHA, Content: result})
		actual.Files = append(actual.Files, repositoryActualFileDiff{
			Path: path, BaseSHA: source.SHA, BeforeSHA256: repositoryContentHash(original), AfterSHA256: repositoryContentHash(result), Changes: changes,
		})
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].Path < updates[j].Path })
	sort.Slice(actual.Files, func(i, j int) bool { return actual.Files[i].Path < actual.Files[j].Path })
	diff, err := json.Marshal(actual)
	if err != nil {
		return nil, nil, err
	}
	return updates, diff, nil
}

func ValidateRepositorySnapshot(snapshot RepositorySnapshot) error {
	if strings.TrimSpace(snapshot.Repo) == "" || strings.TrimSpace(snapshot.Branch) == "" || strings.TrimSpace(snapshot.BaseCommitSHA) == "" {
		return errors.New("repository snapshot target is incomplete")
	}
	if len(snapshot.Sources) == 0 || len(snapshot.Sources) > MaxRepositorySourceFiles {
		return fmt.Errorf("repository snapshot must contain between one and %d sources", MaxRepositorySourceFiles)
	}
	seen := make(map[string]struct{}, len(snapshot.Sources))
	total := 0
	for _, source := range snapshot.Sources {
		path, err := validateRepositoryPath(source.Path)
		if err != nil {
			return err
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("repository snapshot contains duplicate path %q", path)
		}
		seen[path] = struct{}{}
		if strings.TrimSpace(source.SHA) == "" {
			return fmt.Errorf("repository source %q is missing its blob sha", path)
		}
		if len(source.Content) > MaxRepositorySourceFileBytes {
			return fmt.Errorf("repository source %q exceeds %d bytes", path, MaxRepositorySourceFileBytes)
		}
		if err := validateRepositoryText(source.Content, "source content"); err != nil {
			return fmt.Errorf("repository source %q: %w", path, err)
		}
		if err := validateRepositorySourceContentSafety(source.Content); err != nil {
			return fmt.Errorf("repository source %q: %w", path, err)
		}
		total += len(source.Content)
		if total > MaxRepositorySourceTotalBytes {
			return fmt.Errorf("repository source snapshot exceeds %d bytes", MaxRepositorySourceTotalBytes)
		}
	}
	return nil
}

func BuildRepositoryPreparedPatch(snapshot RepositorySnapshot, patch RepositoryPatch, updates []RepositoryFileUpdate) (json.RawMessage, error) {
	if len(updates) != len(patch.Files) {
		return nil, errors.New("prepared patch update count does not match patch")
	}
	sourceByPath := make(map[string]RepositorySource, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		sourceByPath[source.Path] = source
	}
	updateByPath := make(map[string]RepositoryFileUpdate, len(updates))
	for _, update := range updates {
		updateByPath[update.Path] = update
	}
	prepared := RepositoryPreparedPatch{Repo: snapshot.Repo, BaseBranch: snapshot.Branch, BaseCommitSHA: snapshot.BaseCommitSHA}
	var sourceAggregate, resultAggregate bytes.Buffer
	files := append([]RepositoryFilePatch(nil), patch.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		source, sourceOK := sourceByPath[file.Path]
		update, updateOK := updateByPath[file.Path]
		if !sourceOK || !updateOK || source.SHA != file.BaseSHA || update.BaseSHA != file.BaseSHA {
			return nil, fmt.Errorf("prepared patch identity mismatch for %q", file.Path)
		}
		sourceHash, resultHash := repositoryContentHash([]byte(source.Content)), repositoryContentHash(update.Content)
		prepared.Files = append(prepared.Files, RepositoryPreparedFilePatch{
			Path: file.Path, BaseSHA: file.BaseSHA, SourceContentSHA256: sourceHash, ResultContentSHA256: resultHash,
			Replacements: append([]ExactReplacement(nil), file.Replacements...),
		})
		sourceAggregate.WriteString(file.Path + "\x00" + file.BaseSHA + "\x00" + sourceHash + "\n")
		resultAggregate.WriteString(file.Path + "\x00" + resultHash + "\n")
	}
	prepared.SourceAggregateSHA256 = repositoryContentHash(sourceAggregate.Bytes())
	prepared.ResultAggregateSHA256 = repositoryContentHash(resultAggregate.Bytes())
	return json.Marshal(prepared)
}

// ParseRepositoryPreparedPatch validates the immutable identities and hashes
// in a durable prepared artifact. Additional audited metadata fields are
// allowed because the generator attaches finding-family context at top level.
func ParseRepositoryPreparedPatch(raw json.RawMessage) (RepositoryPreparedPatch, error) {
	if !json.Valid(raw) {
		return RepositoryPreparedPatch{}, errors.New("prepared repository patch is invalid JSON")
	}
	var prepared RepositoryPreparedPatch
	if err := json.Unmarshal(raw, &prepared); err != nil {
		return RepositoryPreparedPatch{}, errors.New("prepared repository patch is invalid")
	}
	if prepared.Repo == "" || strings.TrimSpace(prepared.Repo) != prepared.Repo ||
		prepared.BaseBranch == "" || strings.TrimSpace(prepared.BaseBranch) != prepared.BaseBranch ||
		prepared.BaseCommitSHA == "" || strings.TrimSpace(prepared.BaseCommitSHA) != prepared.BaseCommitSHA {
		return RepositoryPreparedPatch{}, errors.New("prepared repository patch target is incomplete")
	}
	if len(prepared.Files) == 0 || len(prepared.Files) > MaxRepositorySourceFiles {
		return RepositoryPreparedPatch{}, fmt.Errorf("prepared repository patch must contain between one and %d files", MaxRepositorySourceFiles)
	}
	if !validRepositoryContentHash(prepared.SourceAggregateSHA256) || !validRepositoryContentHash(prepared.ResultAggregateSHA256) {
		return RepositoryPreparedPatch{}, errors.New("prepared repository patch aggregate hashes are invalid")
	}
	seen := make(map[string]struct{}, len(prepared.Files))
	for _, file := range prepared.Files {
		path, err := validateRepositoryPath(file.Path)
		if err != nil {
			return RepositoryPreparedPatch{}, err
		}
		if _, duplicate := seen[path]; duplicate {
			return RepositoryPreparedPatch{}, fmt.Errorf("prepared repository patch contains duplicate path %q", path)
		}
		seen[path] = struct{}{}
		if file.BaseSHA == "" || strings.TrimSpace(file.BaseSHA) != file.BaseSHA ||
			!validRepositoryContentHash(file.SourceContentSHA256) || !validRepositoryContentHash(file.ResultContentSHA256) || len(file.Replacements) == 0 {
			return RepositoryPreparedPatch{}, fmt.Errorf("prepared repository patch identity is invalid for %q", path)
		}
	}
	return prepared, nil
}

// ReapplyRepositoryPreparedPatch reconstructs final file contents from a
// durable prepared artifact and freshly re-read immutable blobs. It verifies
// every file and aggregate hash before returning publisher-ready updates.
func ReapplyRepositoryPreparedPatch(raw json.RawMessage, snapshot RepositorySnapshot) ([]RepositoryFileUpdate, json.RawMessage, RepositoryPreparedPatch, error) {
	prepared, err := ParseRepositoryPreparedPatch(raw)
	if err != nil {
		return nil, nil, RepositoryPreparedPatch{}, err
	}
	if err := ValidateRepositorySnapshot(snapshot); err != nil {
		return nil, nil, RepositoryPreparedPatch{}, err
	}
	if prepared.Repo != snapshot.Repo || prepared.BaseBranch != snapshot.Branch || prepared.BaseCommitSHA != snapshot.BaseCommitSHA {
		return nil, nil, RepositoryPreparedPatch{}, errors.New("prepared repository patch target does not match the source snapshot")
	}
	if len(prepared.Files) == 0 || len(prepared.Files) > MaxRepositorySourceFiles || len(prepared.Files) != len(snapshot.Sources) {
		return nil, nil, RepositoryPreparedPatch{}, errors.New("prepared repository patch file set does not match the source snapshot")
	}
	sources := make(map[string]RepositorySource, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		sources[source.Path] = source
	}
	patch := RepositoryPatch{Files: make([]RepositoryFilePatch, 0, len(prepared.Files))}
	for _, file := range prepared.Files {
		source, ok := sources[file.Path]
		if !ok || source.SHA != file.BaseSHA || repositoryContentHash([]byte(source.Content)) != file.SourceContentSHA256 {
			return nil, nil, RepositoryPreparedPatch{}, fmt.Errorf("prepared repository patch source identity mismatch for %q", file.Path)
		}
		patch.Files = append(patch.Files, RepositoryFilePatch{
			Path: file.Path, BaseSHA: file.BaseSHA, Replacements: append([]ExactReplacement(nil), file.Replacements...),
		})
	}
	updates, actualDiff, err := ApplyRepositoryPatch(snapshot, patch)
	if err != nil {
		return nil, nil, RepositoryPreparedPatch{}, err
	}
	rebuiltRaw, err := BuildRepositoryPreparedPatch(snapshot, patch, updates)
	if err != nil {
		return nil, nil, RepositoryPreparedPatch{}, err
	}
	var rebuilt RepositoryPreparedPatch
	if err := json.Unmarshal(rebuiltRaw, &rebuilt); err != nil || !reflect.DeepEqual(prepared, rebuilt) {
		return nil, nil, RepositoryPreparedPatch{}, errors.New("prepared repository patch hashes do not match reconstructed contents")
	}
	return updates, actualDiff, prepared, nil
}

// PreserveRepositoryActualDiffMetadata verifies that a previously grounded
// diff has the same locally-recomputed file changes, then copies only the
// scheduler metadata fields produced by repositoryApplicationMetadata.
func PreserveRepositoryActualDiffMetadata(persisted, actual json.RawMessage) (json.RawMessage, error) {
	var persistedObject, actualObject map[string]any
	if json.Unmarshal(persisted, &persistedObject) != nil || persistedObject == nil ||
		json.Unmarshal(actual, &actualObject) != nil || actualObject == nil {
		return nil, errors.New("repository actual diff is invalid")
	}
	persistedFiles, persistedOK := persistedObject["files"]
	actualFiles, actualOK := actualObject["files"]
	if !persistedOK || !actualOK || !reflect.DeepEqual(persistedFiles, actualFiles) {
		return nil, errors.New("persisted repository diff does not match locally reconstructed file changes")
	}
	for _, key := range []string{"asset_type", "proposed_change", "proposed_metadata", "proposed_value", "proposed_title", "proposed_meta_description", "field"} {
		if value, ok := persistedObject[key]; ok {
			actualObject[key] = value
		}
	}
	return json.Marshal(actualObject)
}

func validRepositoryContentHash(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

var repositorySecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{20,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
	regexp.MustCompile(`sk_live_[0-9A-Za-z]{16,}`),
	regexp.MustCompile(`npm_[0-9A-Za-z]{20,}`),
}

var (
	repositoryCredentialAssignmentPattern      = regexp.MustCompile(`(?i)(?:^|[ \t{(\[,;])["']?([a-z][a-z0-9_.-]*)["']?[ \t]*(?::=|=>|=|:)[ \t]*`)
	repositoryTypedCredentialAssignmentPattern = regexp.MustCompile(`(?i)^(?:string|str|bytes|secretstring)[ \t]*=[ \t]*(.+)$`)
	repositoryEnvironmentReferencePattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	repositoryIdentifierReferencePattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func validateRepositorySourceContentSafety(content string) error {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "-----begin private key-----") ||
		strings.Contains(lower, "-----begin rsa private key-----") ||
		strings.Contains(lower, "-----begin ec private key-----") ||
		strings.Contains(lower, "-----begin openssh private key-----") {
		return errors.New("repository source contains private key material")
	}
	for _, pattern := range repositorySecretPatterns {
		if pattern.FindStringIndex(content) != nil {
			return errors.New("repository source contains a known credential pattern")
		}
	}
	if containsLiteralRepositoryCredentialAssignment(content) {
		return errors.New("repository source contains a literal credential assignment")
	}
	if strings.Contains(content, "DO NOT EDIT") ||
		strings.Contains(lower, "code generated") ||
		strings.Contains(lower, "@generated") {
		return errors.New("repository source is generated")
	}
	if looksMinifiedRepositorySource(content) {
		return errors.New("repository source appears minified")
	}
	return nil
}

func containsLiteralRepositoryCredentialAssignment(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		matches := repositoryCredentialAssignmentPattern.FindAllStringSubmatchIndex(line, -1)
		for i, match := range matches {
			if len(match) < 4 || !sensitiveRepositoryCredentialKey(line[match[2]:match[3]]) {
				continue
			}
			valueEnd := len(line)
			if i+1 < len(matches) {
				valueEnd = matches[i+1][0]
			}
			if literalRepositoryCredentialValue(line[match[1]:valueEnd]) {
				return true
			}
		}
	}
	return false
}

func sensitiveRepositoryCredentialKey(key string) bool {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, suffix := range []string{"password", "passwd", "pwd", "token", "apikey", "secret", "privatekey", "credential", "credentials"} {
		if strings.HasSuffix(compact, suffix) {
			return true
		}
	}
	return false
}

func literalRepositoryCredentialValue(raw string) bool {
	value := trimRepositoryCredentialAssignmentValue(raw)
	if value == "" {
		return false
	}
	if annotated := repositoryTypedCredentialAssignmentPattern.FindStringSubmatch(value); annotated != nil {
		return literalRepositoryCredentialValue(annotated[1])
	}
	if quoted, ok := repositoryQuotedAssignmentValue(value); ok {
		return strings.TrimSpace(quoted) != "" &&
			!repositoryCredentialPlaceholder(quoted) &&
			!repositoryCredentialRuntimeReference(quoted)
	}
	lower := strings.ToLower(value)
	if lower == "null" || lower == "nil" || lower == "none" || lower == "undefined" {
		return false
	}
	if repositoryCredentialPlaceholder(value) || repositoryCredentialRuntimeReference(value) {
		return false
	}
	// Unquoted values containing whitespace are usually prose, type unions, or
	// structured expressions rather than the simple scalar assignments guarded
	// here. Quoted values remain fail-closed regardless of whitespace.
	return !strings.ContainsAny(value, " \t")
}

func trimRepositoryCredentialAssignmentValue(raw string) string {
	value := strings.TrimSpace(raw)
	for _, marker := range []string{" #", " //"} {
		if index := strings.Index(value, marker); index >= 0 {
			value = value[:index]
		}
	}
	value = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(value), ",;"))
	for value != "" {
		last := value[len(value)-1]
		if (last != '}' || strings.HasPrefix(value, "{") || strings.HasPrefix(value, "${")) &&
			(last != ']' || strings.HasPrefix(value, "[")) {
			break
		}
		value = strings.TrimSpace(value[:len(value)-1])
	}
	return value
}

func repositoryQuotedAssignmentValue(value string) (string, bool) {
	if len(value) == 0 || (value[0] != '\'' && value[0] != '"' && value[0] != '`') {
		return "", false
	}
	quote := value[0]
	escaped := false
	for i := 1; i < len(value); i++ {
		if escaped {
			escaped = false
			continue
		}
		if value[i] == '\\' {
			escaped = true
			continue
		}
		if value[i] == quote {
			return value[1:i], true
		}
	}
	return value[1:], true
}

func repositoryCredentialPlaceholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	if (strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">")) ||
		(strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}")) ||
		(strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "$") && repositoryEnvironmentReferencePattern.MatchString(trimmed[1:])) {
		return true
	}
	marker := strings.Trim(strings.ToLower(trimmed), "_-.[]() ")
	switch marker {
	case "placeholder", "redacted", "omitted", "unset", "notset", "todo", "changeme", "change-me", "change_me", "replaceme", "replace-me", "replace_me":
		return true
	}
	if strings.HasPrefix(marker, "your_") || strings.HasPrefix(marker, "your-") || strings.HasPrefix(marker, "placeholder_") {
		return true
	}
	for _, r := range marker {
		if r != 'x' && r != '*' {
			return false
		}
	}
	return marker != ""
}

func repositoryCredentialRuntimeReference(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	for _, fragment := range []string{
		"process.env.", "import.meta.env.", "os.getenv(", "os.environ[", "getenv(", "env(",
		"settings.", "config.", "secrets.", "vault.", "getsecret(", "loadsecret(",
	} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	if repositoryEnvironmentReferencePattern.MatchString(trimmed) {
		return true
	}
	if repositoryCredentialTypeReference(lower) {
		return true
	}
	if repositoryIdentifierReferencePattern.MatchString(trimmed) {
		if sensitiveRepositoryCredentialKey(trimmed) {
			return true
		}
		for _, r := range trimmed[1:] {
			if unicode.IsUpper(r) {
				return true
			}
		}
	}
	return false
}

func repositoryCredentialTypeReference(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "string", "str", "bytes", "secretstring":
		return true
	default:
		return false
	}
}

func looksMinifiedRepositorySource(content string) bool {
	if len(content) < 8*1024 {
		return false
	}
	for _, line := range strings.Split(content, "\n") {
		if len(line) < 8*1024 {
			continue
		}
		spaces := 0
		for _, r := range line {
			if unicode.IsSpace(r) {
				spaces++
			}
		}
		if spaces*5 < len(line) {
			return true
		}
	}
	return false
}

func validateRepositoryText(value, label string) error {
	if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s must be UTF-8 text without NUL bytes", label)
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("%s must not contain binary control characters", label)
		}
	}
	return nil
}

func validateRepositoryPath(path string) (string, error) {
	if path == "" || strings.TrimSpace(path) != path || strings.HasPrefix(path, "/") || strings.Contains(path, "\\") {
		return "", fmt.Errorf("invalid repository path %q", path)
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("invalid repository path %q", path)
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid repository path %q", path)
		}
	}
	return path, nil
}

func repositoryContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
