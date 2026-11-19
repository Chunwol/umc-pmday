package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChallengerRosterEndpointIsNotPublic(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/challengers", nil)
	response := httptest.NewRecorder()
	var recovered any

	func() {
		defer func() { recovered = recover() }()
		(&Server{}).Routes().ServeHTTP(response, request)
	}()

	if recovered != nil {
		t.Fatalf("GET /api/challengers is still routed: %v", recovered)
	}
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /api/challengers returned %d, want %d", response.Code, http.StatusNotFound)
	}
}
