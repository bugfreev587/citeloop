package api

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestGrowthRadarRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	want := map[string]bool{"GET /api/projects/{projectID}/opportunities/radar": false, "GET /api/projects/{projectID}/seo/opportunity-finding/radar": false}
	if err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, ok := want[method+" "+route]; ok {
			want[method+" "+route] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for route, found := range want {
		if !found {
			t.Errorf("missing route %s", route)
		}
	}
}
