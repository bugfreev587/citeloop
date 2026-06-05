package config

import (
	"encoding/json"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	c, err := Parse(json.RawMessage("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if c.BufferDays != 5 || c.Crawl.MaxPages != 200 {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

// Regression: an explicit buffer_days:0 must be honored, not coerced to default.
func TestParseExplicitZeroBuffer(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"buffer_days":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.BufferDays != 0 {
		t.Fatalf("buffer_days:0 was coerced to %d", c.BufferDays)
	}
	// absent crawl bounds still keep defaults
	if c.Crawl.MaxPages != 200 {
		t.Fatalf("absent crawl.max_pages lost its default: %d", c.Crawl.MaxPages)
	}
}

// Partial nested config must preserve sibling defaults.
func TestParsePartialCrawl(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"crawl":{"max_pages":50}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Crawl.MaxPages != 50 {
		t.Fatalf("explicit max_pages lost: %d", c.Crawl.MaxPages)
	}
	if c.Crawl.MaxDepth != 3 {
		t.Fatalf("sibling crawl.max_depth default lost: %d", c.Crawl.MaxDepth)
	}
}
