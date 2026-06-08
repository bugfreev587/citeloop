package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		name string
		path string
		want int
	}{
		{name: "bad project", path: "/api/projects/not-a-uuid/runs", want: http.StatusBadRequest},
		{name: "bad run", path: "/api/projects/11111111-1111-1111-1111-111111111111/runs/not-a-run", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status = %d, want %d", res.Code, tc.want)
			}
		})
	}
}

func TestSanitizeRunPayloadRedactsSensitiveMaterial(t *testing.T) {
	payload := sanitizeRunPayload([]byte(`{
		"token":"ghp_secret",
		"nested":{"deploy_hook_url":"https://api.vercel.com/v1/integrations/deploy/prj/secret"},
		"items":["https://discord.com/api/webhooks/1/secret","safe value"]
	}`))

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"ghp_secret", "api.vercel.com", "discord.com/api/webhooks"} {
		if strings.Contains(string(body), leak) {
			t.Fatalf("payload leaked %q: %s", leak, string(body))
		}
	}
	if !strings.Contains(string(body), "[redacted]") {
		t.Fatalf("payload did not include redaction marker: %s", string(body))
	}
}
