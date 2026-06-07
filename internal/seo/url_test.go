package seo

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		site    string
		cfg     URLNormalizationConfig
		want    string
		wantErr bool
	}{
		{
			name: "absolute URL drops tracking and trailing slash",
			raw:  "HTTP://Example.COM/blog/post/?utm_source=x&fbclid=y#section",
			site: "https://example.com",
			want: "https://example.com/blog/post",
		},
		{
			name: "GA4 path joins site URL",
			raw:  "/blog/post/?utm_campaign=x",
			site: "https://example.com",
			want: "https://example.com/blog/post",
		},
		{
			name: "keeps whitelisted query keys deterministically",
			raw:  "https://example.com/docs?page=2&utm_source=x&lang=en",
			site: "https://example.com",
			cfg:  URLNormalizationConfig{KeepQueryKeys: []string{"lang", "page"}},
			want: "https://example.com/docs?lang=en&page=2",
		},
		{
			name: "preserves path case by default",
			raw:  "https://example.com/Blog/Post",
			site: "https://example.com",
			want: "https://example.com/Blog/Post",
		},
		{
			name: "lowercases path when configured",
			raw:  "https://example.com/Blog/Post",
			site: "https://example.com",
			cfg:  URLNormalizationConfig{LowercasePath: true},
			want: "https://example.com/blog/post",
		},
		{
			name: "preserves http when configured",
			raw:  "http://example.com/blog/post/",
			site: "http://example.com",
			cfg:  URLNormalizationConfig{PreserveHTTP: true},
			want: "http://example.com/blog/post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.raw, tt.site, tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeURL error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeURL = %q, want %q", got, tt.want)
			}
		})
	}
}
