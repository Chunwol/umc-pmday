package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"umc-pmday/internal/db"
	"umc-pmday/internal/models"
)

func TestAttendanceCancellationValidation(t *testing.T) {
	valid := models.AttendanceCancellationRequest{Nickname: "tester", Name: "Test", School: "Example", Phone: "010-0000-0000", Reason: "Required class conflicts with the event"}
	cutoff := time.Date(2026, 11, 21, 0, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	for _, tc := range []struct {
		name   string
		req    models.AttendanceCancellationRequest
		now    time.Time
		wantOK bool
	}{
		{"just before deadline", valid, cutoff.Add(-time.Nanosecond), true},
		{"at deadline", valid, cutoff, false},
		{"after deadline UTC", valid, cutoff.Add(time.Second).UTC(), false},
		{"before deadline UTC", valid, cutoff.Add(-time.Second).UTC(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateAttendanceCancellation(tc.req, tc.now); (got == "") != tc.wantOK {
				t.Fatalf("message = %q, wantOK %v", got, tc.wantOK)
			}
		})
	}
	for _, reason := range []string{"", " \n ", strings.Repeat("가", 2001)} {
		req := valid
		req.Reason = reason
		if validateAttendanceCancellation(req, cutoff.Add(-time.Hour)) == "" {
			t.Fatal("invalid reason accepted")
		}
	}
	valid.Phone = "1234"
	if validateAttendanceCancellation(valid, cutoff.Add(-time.Hour)) == "" {
		t.Fatal("partial phone accepted")
	}
}

func TestAttendanceAdminRequiresPIN(t *testing.T) {
	s := &Server{Store: &db.Store{AdminPIN: "test-admin"}}
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/attendance-cancellations", strings.NewReader(`{"pin":"wrong"}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}
