package sitefix

import (
	"encoding/json"
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
