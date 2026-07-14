package api

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPlatformContractRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	want := []string{
		"GET /api/projects/{projectID}/platform-contracts/capabilities",
		"GET /api/projects/{projectID}/platform-target-contexts",
		"POST /api/projects/{projectID}/platform-target-contexts",
		"POST /api/projects/{projectID}/platform-target-contexts/{contextID}/reconfirm",
		"POST /api/projects/{projectID}/articles/{articleID}/target-context",
	}
	registered := make(map[string]bool)
	if err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("platform contract route is not registered: %s", route)
		}
	}
}
