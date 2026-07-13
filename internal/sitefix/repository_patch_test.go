package sitefix

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestApplyRepositoryPatchReturnsFinalContentsAndActualDiff(t *testing.T) {
	snapshot := RepositorySnapshot{
		Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit-1",
		Sources: []RepositorySource{{
			Path: "app/sitemap.ts", SHA: "sha-1",
			Content: "export default function sitemap() {\n  return []\n}\n",
		}},
	}
	patch := RepositoryPatch{Files: []RepositoryFilePatch{{
		Path: "app/sitemap.ts", BaseSHA: "sha-1",
		Replacements: []ExactReplacement{{OldText: "return []", NewText: "return [{ url: canonicalURL }]"}},
	}}}

	updates, diff, err := ApplyRepositoryPatch(snapshot, patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || !strings.Contains(string(updates[0].Content), "canonicalURL") {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].Path != "app/sitemap.ts" || updates[0].BaseSHA != "sha-1" {
		t.Fatalf("update identity = %#v", updates[0])
	}
	var actual struct {
		Files []struct {
			Path    string `json:"path"`
			Changes []struct {
				Before string `json:"before"`
				After  string `json:"after"`
			} `json:"changes"`
		} `json:"files"`
	}
	if err := json.Unmarshal(diff, &actual); err != nil {
		t.Fatalf("diff is not JSON: %v (%s)", err, diff)
	}
	if len(actual.Files) != 1 || actual.Files[0].Path != "app/sitemap.ts" ||
		len(actual.Files[0].Changes) != 1 || actual.Files[0].Changes[0].Before != "return []" ||
		actual.Files[0].Changes[0].After != "return [{ url: canonicalURL }]" {
		t.Fatalf("actual diff = %s", diff)
	}
}

func TestApplyRepositoryPatchRejectsUnsafeOrAmbiguousChanges(t *testing.T) {
	base := RepositorySnapshot{
		Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit-1",
		Sources: []RepositorySource{
			{Path: "app/page.tsx", SHA: "sha-page", Content: "<main>same same</main>"},
			{Path: "app/layout.tsx", SHA: "sha-layout", Content: "abcdef"},
		},
	}
	tests := []struct {
		name  string
		patch RepositoryPatch
	}{
		{name: "unknown path", patch: RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/missing.tsx", BaseSHA: "sha", Replacements: []ExactReplacement{{OldText: "a", NewText: "b"}}}}}},
		{name: "sha mismatch", patch: RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/page.tsx", BaseSHA: "other", Replacements: []ExactReplacement{{OldText: "same same", NewText: "changed"}}}}}},
		{name: "duplicate path", patch: RepositoryPatch{Files: []RepositoryFilePatch{
			{Path: "app/layout.tsx", BaseSHA: "sha-layout", Replacements: []ExactReplacement{{OldText: "abc", NewText: "ABC"}}},
			{Path: "app/layout.tsx", BaseSHA: "sha-layout", Replacements: []ExactReplacement{{OldText: "def", NewText: "DEF"}}},
		}}},
		{name: "missing old text", patch: RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/page.tsx", BaseSHA: "sha-page", Replacements: []ExactReplacement{{OldText: "missing", NewText: "changed"}}}}}},
		{name: "ambiguous old text", patch: RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/page.tsx", BaseSHA: "sha-page", Replacements: []ExactReplacement{{OldText: "same", NewText: "changed"}}}}}},
		{name: "overlapping replacements", patch: RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/layout.tsx", BaseSHA: "sha-layout", Replacements: []ExactReplacement{
			{OldText: "abcd", NewText: "ABCD"}, {OldText: "cdef", NewText: "CDEF"},
		}}}}},
		{name: "unchanged replacement", patch: RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/layout.tsx", BaseSHA: "sha-layout", Replacements: []ExactReplacement{{OldText: "abc", NewText: "abc"}}}}}},
		{name: "empty patch", patch: RepositoryPatch{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ApplyRepositoryPatch(base, tc.patch); err == nil {
				t.Fatal("unsafe patch was accepted")
			}
		})
	}
}

func TestApplyRepositoryPatchRejectsExcessiveReplacementWork(t *testing.T) {
	markers := make([]string, MaxRepositoryReplacementsPerFile+1)
	replacements := make([]ExactReplacement, len(markers))
	for i := range markers {
		markers[i] = fmt.Sprintf("marker-%03d", i)
		replacements[i] = ExactReplacement{OldText: markers[i], NewText: fmt.Sprintf("updated-%03d", i)}
	}
	snapshot := RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob", Content: strings.Join(markers, "\n")}}}
	if _, _, err := ApplyRepositoryPatch(snapshot, RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/page.tsx", BaseSHA: "blob", Replacements: replacements}}}); err == nil {
		t.Fatal("excessive per-file replacement count was accepted")
	}

	huge := strings.Repeat("n", MaxRepositoryReplacementBytesPerFile+1)
	snapshot.Sources[0].Content = "old"
	if _, _, err := ApplyRepositoryPatch(snapshot, RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/page.tsx", BaseSHA: "blob", Replacements: []ExactReplacement{{OldText: "old", NewText: huge}}}}}); err == nil {
		t.Fatal("oversized replacement text was accepted")
	}

	files := make([]RepositoryFilePatch, 3)
	sources := make([]RepositorySource, 3)
	remaining := MaxRepositoryReplacementsTotal + 1
	for i := range files {
		count := min(MaxRepositoryReplacementsPerFile, remaining)
		remaining -= count
		parts := make([]string, count)
		fileReplacements := make([]ExactReplacement, count)
		for j := 0; j < count; j++ {
			parts[j] = fmt.Sprintf("f%d-marker-%03d", i, j)
			fileReplacements[j] = ExactReplacement{OldText: parts[j], NewText: fmt.Sprintf("f%d-new-%03d", i, j)}
		}
		path := fmt.Sprintf("app/file-%d.tsx", i)
		sources[i] = RepositorySource{Path: path, SHA: fmt.Sprintf("blob-%d", i), Content: strings.Join(parts, "\n")}
		files[i] = RepositoryFilePatch{Path: path, BaseSHA: sources[i].SHA, Replacements: fileReplacements}
	}
	if _, _, err := ApplyRepositoryPatch(RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: sources}, RepositoryPatch{Files: files}); err == nil {
		t.Fatal("excessive total replacement count was accepted")
	}
}

func TestApplyRepositoryPatchRejectsBinaryControlsButAllowsTextWhitespace(t *testing.T) {
	snapshot := RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob", Content: "old"}}}
	patch := func(newText string) RepositoryPatch {
		return RepositoryPatch{Files: []RepositoryFilePatch{{Path: "app/page.tsx", BaseSHA: "blob", Replacements: []ExactReplacement{{OldText: "old", NewText: newText}}}}}
	}
	if _, _, err := ApplyRepositoryPatch(snapshot, patch("new\x02value")); err == nil {
		t.Fatal("binary control character in replacement was accepted")
	}
	updates, _, err := ApplyRepositoryPatch(snapshot, patch("new\n\tvalue\r\n"))
	if err != nil || len(updates) != 1 || string(updates[0].Content) != "new\n\tvalue\r\n" {
		t.Fatalf("ordinary text whitespace replacement failed: updates=%#v err=%v", updates, err)
	}
}

func TestApplyRepositoryPatchEnforcesTextAndSizeBounds(t *testing.T) {
	validPatch := func(path, sha, old string) RepositoryPatch {
		return RepositoryPatch{Files: []RepositoryFilePatch{{Path: path, BaseSHA: sha, Replacements: []ExactReplacement{{OldText: old, NewText: "changed"}}}}}
	}
	tests := []struct {
		name     string
		snapshot RepositorySnapshot
		patch    RepositoryPatch
	}{
		{
			name: "invalid utf8", snapshot: RepositorySnapshot{Repo: "a/b", Branch: "main", BaseCommitSHA: "c", Sources: []RepositorySource{{Path: "app/a.ts", SHA: "s", Content: string([]byte{0xff, 'a'})}}},
			patch: validPatch("app/a.ts", "s", "a"),
		},
		{
			name: "nul source", snapshot: RepositorySnapshot{Repo: "a/b", Branch: "main", BaseCommitSHA: "c", Sources: []RepositorySource{{Path: "app/a.ts", SHA: "s", Content: "a\x00b"}}},
			patch: validPatch("app/a.ts", "s", "a"),
		},
		{
			name: "oversized source", snapshot: RepositorySnapshot{Repo: "a/b", Branch: "main", BaseCommitSHA: "c", Sources: []RepositorySource{{Path: "app/a.ts", SHA: "s", Content: strings.Repeat("a", MaxRepositorySourceFileBytes+1)}}},
			patch: validPatch("app/a.ts", "s", strings.Repeat("a", MaxRepositorySourceFileBytes+1)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ApplyRepositoryPatch(tc.snapshot, tc.patch); err == nil {
				t.Fatal("unsafe source was accepted")
			}
		})
	}

	sources := make([]RepositorySource, MaxRepositorySourceFiles+1)
	files := make([]RepositoryFilePatch, MaxRepositorySourceFiles+1)
	for i := range sources {
		path := "app/file" + string(rune('a'+i)) + ".ts"
		sources[i] = RepositorySource{Path: path, SHA: path, Content: "old"}
		files[i] = RepositoryFilePatch{Path: path, BaseSHA: path, Replacements: []ExactReplacement{{OldText: "old", NewText: "new"}}}
	}
	if _, _, err := ApplyRepositoryPatch(RepositorySnapshot{Repo: "a/b", Branch: "main", BaseCommitSHA: "c", Sources: sources}, RepositoryPatch{Files: files}); err == nil {
		t.Fatal("more than eight sources/files were accepted")
	}
}

func TestValidateRepositorySnapshotRejectsSensitiveGeneratedOrMinifiedContent(t *testing.T) {
	unsafe := map[string]string{
		"pem private key":     "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
		"github token":        "const leaked = \"ghp_" + strings.Repeat("a", 36) + "\"",
		"github fine grained": "github_pat_" + strings.Repeat("a", 30),
		"aws access key":      "AKIAABCDEFGHIJKLMNOP",
		"slack token":         strings.Join([]string{"xoxb", "1234567890", "1234567890", "abcdefghijklmnop"}, "-"),
		"google api key":      "AIza" + strings.Repeat("A", 35),
		"service account key": `{"type":"service_account","private_key":"-----BEGIN PRIVATE KEY-----\\nsecret"}`,
		"generated marker":    "// Code generated by schema compiler. DO NOT EDIT.\npackage schema",
		"do not edit marker":  "/* THIS FILE IS AUTO-GENERATED. DO NOT EDIT */",
		"minified source":     "(()=>{" + strings.Repeat("const a=1;", 3000) + "})();",
		"binary control":      "export const value = \"bad\x01value\"",
	}
	for name, content := range unsafe {
		t.Run(name, func(t *testing.T) {
			snapshot := RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob", Content: content}}}
			if err := ValidateRepositorySnapshot(snapshot); err == nil {
				t.Fatal("unsafe repository content was accepted")
			}
		})
	}
	ordinary := RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: []RepositorySource{{
		Path: "app/page.tsx", SHA: "blob", Content: "export default function Page() {\n\tconst tokenCount = 3\n\treturn <main>Please do not edit settings while syncing. Auto-generated report previews help teams.</main>\n}\n",
	}}}
	if err := ValidateRepositorySnapshot(ordinary); err != nil {
		t.Fatalf("ordinary page source was rejected: %v", err)
	}
}

func TestValidateRepositorySnapshotRejectsLiteralCredentialAssignments(t *testing.T) {
	unsafe := map[string]string{
		"uppercase shell password": `DATABASE_PASSWORD='hunter2'`,
		"hyphenated yaml secret":   `client-secret: value`,
		"dotted toml api key":      `api.key = "opaque-live-key"`,
		"json private key":         `{"private_key":"opaque-private-material"}`,
		"camel case token":         `const accessToken = "ordinary-looking-token";`,
		"numeric password":         `password: 123456`,
		"typed password literal":   `const password: string = "hunter2";`,
	}
	for name, content := range unsafe {
		t.Run(name, func(t *testing.T) {
			snapshot := RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: []RepositorySource{{Path: "app/settings.ts", SHA: "blob", Content: content}}}
			if err := ValidateRepositorySnapshot(snapshot); err == nil {
				t.Fatalf("literal credential assignment was accepted: %s", content)
			}
		})
	}
}

func TestValidateRepositorySnapshotAllowsCredentialPlaceholdersReferencesAndProse(t *testing.T) {
	allowed := map[string]string{
		"empty value":          `password: ""`,
		"null value":           `client_secret: null`,
		"shell environment":    `DATABASE_PASSWORD=${DATABASE_PASSWORD}`,
		"javascript env":       `const password = process.env.DATABASE_PASSWORD;`,
		"config reference":     `api_key = settings.API_KEY`,
		"function lookup":      `client_secret: os.getenv("CLIENT_SECRET")`,
		"angle placeholder":    `client-secret: <set-in-secret-manager>`,
		"template placeholder": `private_key: "{{ vault.private_key }}"`,
		"named placeholder":    `api.key = "YOUR_API_KEY"`,
		"redacted placeholder": `token: REDACTED`,
		"type annotation":      `interface Credentials { password: string }`,
		"optional type field":  `interface Credentials { token?: string }`,
		"variable reference":   `const config = { token: generatedToken };`,
		"ordinary prose":       `<p>Password: must be at least 12 characters.</p>`,
		"ordinary url":         `const passwordResetURL = "https://example.com/reset?token=preview";`,
		"schema json":          `{"@context":"https://schema.org","api":"https://example.com/docs"}`,
		"sitemap yaml":         "sitemap:\n  url: https://example.com/sitemap.xml\n",
	}
	for name, content := range allowed {
		t.Run(name, func(t *testing.T) {
			snapshot := RepositorySnapshot{Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit", Sources: []RepositorySource{{Path: "app/settings.ts", SHA: "blob", Content: content}}}
			if err := ValidateRepositorySnapshot(snapshot); err != nil {
				t.Fatalf("safe source was rejected: %v", err)
			}
		})
	}
}

func TestReapplyRepositoryPreparedPatchVerifiesDurableHashes(t *testing.T) {
	snapshot := RepositorySnapshot{
		Repo: "acme/site", Branch: "citeloop-content", BaseCommitSHA: "commit-1",
		Sources: []RepositorySource{
			{Path: "app/layout.tsx", SHA: "blob-layout", Content: "export const title = 'Old'\n"},
			{Path: "app/sitemap.ts", SHA: "blob-sitemap", Content: "return []\n"},
		},
	}
	patch := RepositoryPatch{Files: []RepositoryFilePatch{
		{Path: "app/layout.tsx", BaseSHA: "blob-layout", Replacements: []ExactReplacement{{OldText: "'Old'", NewText: "'New'"}}},
		{Path: "app/sitemap.ts", BaseSHA: "blob-sitemap", Replacements: []ExactReplacement{{OldText: "[]", NewText: "[canonicalURL]"}}},
	}}
	initial, _, err := ApplyRepositoryPatch(snapshot, patch)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := BuildRepositoryPreparedPatch(snapshot, patch, initial)
	if err != nil {
		t.Fatal(err)
	}
	updates, diff, artifact, err := ReapplyRepositoryPreparedPatch(prepared, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || len(artifact.Files) != 2 || !strings.Contains(string(diff), "canonicalURL") {
		t.Fatalf("updates=%#v artifact=%#v diff=%s", updates, artifact, diff)
	}

	tampered := snapshot
	tampered.Sources = append([]RepositorySource(nil), snapshot.Sources...)
	tampered.Sources[0].Content = "export const title = 'Tampered'\n"
	if _, _, _, err := ReapplyRepositoryPreparedPatch(prepared, tampered); err == nil {
		t.Fatal("tampered source content hash was accepted")
	}

	var object map[string]any
	if err := json.Unmarshal(prepared, &object); err != nil {
		t.Fatal(err)
	}
	object["result_aggregate_sha256"] = strings.Repeat("0", 64)
	badAggregate, _ := json.Marshal(object)
	if _, _, _, err := ReapplyRepositoryPreparedPatch(badAggregate, snapshot); err == nil {
		t.Fatal("tampered prepared aggregate hash was accepted")
	}
}

func TestPreserveRepositoryActualDiffMetadataKeepsOnlyApprovedSchedulerFields(t *testing.T) {
	actual := json.RawMessage(`{"files":[{"path":"app/page.tsx","base_sha":"blob-1","before_sha256":"before","after_sha256":"after","changes":[{"before":"Old","after":"New"}]}]}`)
	persisted := json.RawMessage(`{"files":[{"path":"app/page.tsx","base_sha":"blob-1","before_sha256":"before","after_sha256":"after","changes":[{"before":"Old","after":"New"}]}],"asset_type":"metadata_rewrite","proposed_title":"New","proposed_change":{"title":"New"},"untrusted":"drop-me"}`)
	merged, err := PreserveRepositoryActualDiffMetadata(persisted, actual)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), `"asset_type":"metadata_rewrite"`) || !strings.Contains(string(merged), `"proposed_title":"New"`) || !strings.Contains(string(merged), `"proposed_change":{"title":"New"}`) {
		t.Fatalf("scheduler metadata was lost: %s", merged)
	}
	if strings.Contains(string(merged), "untrusted") {
		t.Fatalf("unapproved metadata survived: %s", merged)
	}
	mismatch := json.RawMessage(`{"files":[{"path":"app/other.tsx"}]}`)
	if _, err := PreserveRepositoryActualDiffMetadata(mismatch, actual); err == nil {
		t.Fatal("persisted model diff that differs from the actual reapplication was accepted")
	}
}
