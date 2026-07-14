package api

import (
	"net/http"
	"testing"

	"github.com/citeloop/citeloop/internal/growthstage"
	"github.com/go-chi/chi/v5"
)

func TestGrowthStageRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	want := map[string]bool{
		"GET /api/projects/{projectID}/opportunities/stage": false,
		"PUT /api/projects/{projectID}/opportunities/stage": false,
	}
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

func TestVirtualGrowthStageIsUnconfirmedFoundation(t *testing.T) {
	response := virtualGrowthStageResponse()
	if response.Stage != growthstage.Foundation || response.SettingVersion != 0 || !response.IsDefaultUnconfirmed || len(response.Profiles) != 4 {
		t.Fatalf("response = %+v", response)
	}
}
