package domainutil

import "testing"

func TestRegistrableDomain(t *testing.T) {
	cases := map[string]string{
		"https://app.buffer.com/resources": "buffer.com",
		"www.example.co.uk":                "example.co.uk",
		"127.0.0.1":                        "127.0.0.1",
		"localhost":                        "",
	}
	for value, want := range cases {
		if got := RegistrableDomain(value); got != want {
			t.Fatalf("RegistrableDomain(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestSameRegistrableDomain(t *testing.T) {
	if !SameRegistrableDomain("https://app.buffer.com/a", "buffer.com") {
		t.Fatalf("expected app.buffer.com and buffer.com to match")
	}
	if SameRegistrableDomain("postsyncer.com", "buffer.com") {
		t.Fatalf("expected different domains not to match")
	}
}
