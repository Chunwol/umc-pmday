package capacity

import (
	"testing"
	"umc-pmday/internal/models"
)

func TestComputeFixed45AndAutoEven(t *testing.T) {
	// 1) 86 total across 18 groups -> 14 groups of 5, 4 groups of 4
	settings := models.Settings{TotalQuota: 86, DistributionMode: "auto_even"}
	counts := map[int]int{}
	caps, sum := Compute(settings, counts)
	if sum.BaseCap != 4 || sum.HighCap != 5 || sum.AllowedHighGroups != 14 {
		t.Fatalf("unexpected summary for 86: %+v", sum)
	}
	for id := 1; id <= 18; id++ {
		if caps[id] != 5 {
			t.Fatalf("expected group %d cap=5 initially, got %d", id, caps[id])
		}
	}

	// Fill 14 groups to 5 -> remaining 4 groups must immediately lock to 4
	for id := 1; id <= 14; id++ {
		counts[id] = 5
	}
	counts[15] = 4
	counts[16] = 2
	caps, sum = Compute(settings, counts)
	if sum.RemainingHighSlots != 0 {
		t.Fatalf("expected 0 remaining high slots, got %d", sum.RemainingHighSlots)
	}
	if caps[15] != 4 || caps[16] != 4 || caps[17] != 4 || caps[18] != 4 {
		t.Fatalf("expected remaining groups locked to 4, got 15:%d 16:%d 17:%d 18:%d", caps[15], caps[16], caps[17], caps[18])
	}

	// 2) 75 total in fixed_4_5 -> 3 groups of 5, 15 groups of 4
	settings75 := models.Settings{TotalQuota: 75, DistributionMode: "fixed_4_5"}
	counts75 := map[int]int{1: 5, 2: 5, 3: 5, 4: 4, 5: 3}
	caps75, sum75 := Compute(settings75, counts75)
	if sum75.AllowedHighGroups != 3 || sum75.RemainingHighSlots != 0 {
		t.Fatalf("unexpected summary for 75: %+v", sum75)
	}
	if caps75[4] != 4 || caps75[5] != 4 {
		t.Fatalf("expected group 4 and 5 locked to 4, got %d, %d", caps75[4], caps75[5])
	}

	// 3) 61 total in auto_even -> 7 groups of 4, 11 groups of 3
	settings61 := models.Settings{TotalQuota: 61, DistributionMode: "auto_even"}
	counts61 := map[int]int{}
	for id := 1; id <= 7; id++ {
		counts61[id] = 4
	}
	caps61, sum61 := Compute(settings61, counts61)
	if sum61.BaseCap != 3 || sum61.HighCap != 4 || sum61.AllowedHighGroups != 7 || sum61.RemainingHighSlots != 0 {
		t.Fatalf("unexpected summary for 61: %+v", sum61)
	}
	if caps61[8] != 3 || caps61[18] != 3 {
		t.Fatalf("expected remaining groups locked to 3, got 8:%d 18:%d", caps61[8], caps61[18])
	}
}

func TestLatecomersDoNotReceiveReservedFiveMemberSlots(t *testing.T) {
	settings := models.Settings{TotalQuota: 85, DistributionMode: "auto_even"}
	counts := map[int]int{14: 4, 15: 3, 16: 2, 17: 1}
	for id := 1; id <= 13; id++ {
		counts[id] = 5
	}

	caps, summary := Compute(settings, counts)
	if summary.RemainingHighSlots != 0 {
		t.Fatalf("remaining high slots = %d, want 0", summary.RemainingHighSlots)
	}
	if caps[14] != 4 {
		t.Fatalf("latecomer group cap = %d, want 4 when no five-member slots remain", caps[14])
	}

	// Latecomer status must not reserve unused high-tier seats from regular applicants.
	settings.TotalQuota = 87
	counts = map[int]int{}
	for id := 1; id <= 12; id++ {
		counts[id] = 5
	}
	caps, _ = Compute(settings, counts)
	if caps[13] != 5 {
		t.Fatalf("regular group cap = %d, want 5 when global high-tier slots remain", caps[13])
	}
}
