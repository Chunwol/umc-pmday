package db

import (
	"testing"

	"umc-pmday/internal/models"
)

func TestBuildUnappliedChallengersExcludesAttendanceCancellationRequests(t *testing.T) {
	challengers := []models.Challenger{
		{ID: 1, Nickname: "still-here", Name: "Active", School: "Example", Part: "PE(WEB)", Attendance: "참석"},
		{ID: 2, Nickname: "cancelled", Name: "Request", School: "Example", Part: "PE(Mobile)", Attendance: "참석"},
		{ID: 3, Nickname: "applied", Name: "Member", School: "Example", Part: "PE(WEB)", Attendance: "참석"},
		{ID: 4, Nickname: "absent", Name: "Excluded", School: "Example", Part: "PE(WEB)", Attendance: "불참"},
	}
	appliedKeys := map[string]bool{NormalizeApplicantKey("applied", "Member"): true}
	cancellationKeys := map[string]bool{NormalizeApplicantKey("cancelled", "Request"): true}

	got := buildUnappliedChallengers(challengers, appliedKeys, cancellationKeys)
	if len(got) != 1 {
		t.Fatalf("got %d unapplied challengers, want 1", len(got))
	}
	if got[0].ID != 1 {
		t.Fatalf("unapplied challenger id = %d, want 1", got[0].ID)
	}
}
