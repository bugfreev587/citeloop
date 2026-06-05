package crawl

import (
	"net/url"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"https://Example.com/Blog/Post/":                 "https://example.com/Blog/Post",
		"https://example.com/post?utm_source=x&id=5":     "https://example.com/post?id=5",
		"https://example.com/post#section":               "https://example.com/post",
		"https://example.com/":                           "https://example.com/",
		"https://example.com/a/b/c/?gclid=123&ref=foo":   "https://example.com/a/b/c",
	}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil {
			t.Fatalf("Normalize(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeArticle(t *testing.T) {
	article := []string{
		"https://example.com/blog/my-post",
		"https://example.com/posts/how-to-x",
	}
	notArticle := []string{
		"https://example.com/blog",
		"https://example.com/tag/marketing",
		"https://example.com/author/jane",
		"https://example.com/blog/page/2",
		"https://example.com/feed.xml",
		"https://example.com/",
	}
	for _, u := range article {
		pu, _ := url.Parse(u)
		if !looksLikeArticle(pu) {
			t.Errorf("expected %q to be an article", u)
		}
	}
	for _, u := range notArticle {
		pu, _ := url.Parse(u)
		if looksLikeArticle(pu) {
			t.Errorf("expected %q to NOT be an article", u)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	base, _ := url.Parse("https://example.com/blog")
	same, _ := url.Parse("https://example.com/other")
	diff, _ := url.Parse("https://evil.com/x")
	if !SameOrigin(base, same) {
		t.Error("same host should be same origin")
	}
	if SameOrigin(base, diff) {
		t.Error("different host should not be same origin")
	}
}

func TestRobotsAllowed(t *testing.T) {
	r := &robots{disallow: []string{"/private", "/admin"}}
	if r.allowed("/private/page") {
		t.Error("/private/page should be disallowed")
	}
	if !r.allowed("/blog/post") {
		t.Error("/blog/post should be allowed")
	}
	blocked := &robots{disallow: []string{"/"}}
	if blocked.allowed("/anything") {
		t.Error("disallow / should block everything")
	}
}
