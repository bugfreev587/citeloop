package agents

import (
	"os"
	"strings"
	"testing"
)

func TestWriterPersistsWorkflowLineageInSEOMeta(t *testing.T) {
	typesSource, err := os.ReadFile("types.go")
	if err != nil {
		t.Fatal(err)
	}
	writerSource, err := os.ReadFile("writer.go")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(typesSource) + "\n" + string(writerSource)
	for _, want := range []string{
		"SourceContentActionID",
		"source_content_action_id",
		"applyWorkflowLineageSEOMeta",
		"topic.SourceContentActionID",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("writer must preserve workflow lineage in article seo_meta; missing %q", want)
		}
	}
}
