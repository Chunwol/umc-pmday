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

func TestLatecomerFiveMemberGuarantee(t *testing.T) {
	// Total 87 (15 groups of 5, 3 groups of 4), with 3 latecomers
	settings := models.Settings{TotalQuota: 87, DistributionMode: "auto_even"}

	// Case A: 3 unapplied latecomers -> 12 non-late groups fill to 5, remaining 6 lock at 4 for regulars (total 84 applied, 3 reserved)
	counts := map[int]int{}
	for id := 1; id <= 12; id++ {
		counts[id] = 5
	}
	for id := 13; id <= 18; id++ {
		counts[id] = 4
	}
	caps, sum := ComputeWithLate(settings, counts, nil, 3)
	if sum.RemainingTotal != 3 {
		t.Fatalf("expected 3 reserved seats remaining, got %d", sum.RemainingTotal)
	}
	for id := 13; id <= 18; id++ {
		if caps[id] != 4 {
			t.Fatalf("expected non-late group %d locked at 4 while 3 latecomers unapplied, got %d", id, caps[id])
		}
	}

	// Case B: Latecomer joins early into group 18 (1 member), while groups 1..12 fill to 5
	countsEarly := map[int]int{18: 1}
	for id := 1; id <= 12; id++ {
		countsEarly[id] = 5
	}
	for id := 13; id <= 17; id++ {
		countsEarly[id] = 4
	}
	capsEarly, _ := ComputeWithLate(settings, countsEarly, map[int]bool{18: true}, 2)
	if capsEarly[18] != 5 {
		t.Fatalf("expected latecomer group 18 to guarantee capacity 5, got %d", capsEarly[18])
	}
	if capsEarly[13] != 4 {
		t.Fatalf("expected non-late group 13 locked at 4, got %d", capsEarly[13])
	}
}
