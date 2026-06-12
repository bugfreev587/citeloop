package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestProjectHardDeleteRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+uuid.NewString()+"/", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("DELETE /api/projects/{projectID}/ status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
}
