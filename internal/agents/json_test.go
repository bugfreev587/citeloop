package agents

import "testing"

func TestExtractJSON(t *testing.T) {
	type obj struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	inputs := []string{
		`{"a":"x","b":2}`,
		"Here you go:\n```json\n{\"a\":\"x\",\"b\":2}\n```",
		"prose before {\"a\":\"x\",\"b\":2} prose after",
	}
	for _, in := range inputs {
		var o obj
		if err := extractJSON(in, &o); err != nil {
			t.Fatalf("extractJSON(%q): %v", in, err)
		}
		if o.A != "x" || o.B != 2 {
			t.Errorf("extractJSON(%q) = %+v", in, o)
		}
	}
}

func TestNormalizeChannel(t *testing.T) {
	cases := map[string]string{
		"blog": "blog", "Syndication": "syndication", "BOTH": "both", "garbage": "blog", "": "blog",
	}
	for in, want := range cases {
		if got := normalizeChannel(in); got != want {
			t.Errorf("normalizeChannel(%q) = %q, want %q", in, got, want)
		}
	}
}
