package agents

import "testing"

func TestExtractJSON(t *testing.T) {
	type obj struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	inputs := []struct {
		raw   string
		wantA string
	}{
		{`{"a":"x","b":2}`, "x"},
		{"Here you go:\n```json\n{\"a\":\"x\",\"b\":2}\n```", "x"},
		{"prose before {\"a\":\"x\",\"b\":2} prose after", "x"},
		{"```json\n{\"a\":\"x {template\",\"b\":2}\n```", "x {template"},
	}
	for _, in := range inputs {
		var o obj
		if err := extractJSON(in.raw, &o); err != nil {
			t.Fatalf("extractJSON(%q): %v", in.raw, err)
		}
		if o.A != in.wantA || o.B != 2 {
			t.Errorf("extractJSON(%q) = %+v", in.raw, o)
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
