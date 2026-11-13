package capacity

import (
	"fmt"
	"umc-pmday/internal/models"
)

const GroupCount = 18

// Compute computes per-group effective capacities and summary for the 18 groups.
func Compute(settings models.Settings, countsByProject map[int]int) (map[int]int, models.QuotaSummary) {
	return ComputeWithLate(settings, countsByProject, nil, 0)
}

// ComputeWithLate computes per-group effective capacities while guaranteeing that any group
// containing a latecomer can reach at least 5 members (when highCap >= 5 / totalQuota allows),
// and reserving 5-member group slots for unapplied latecomers so regular applicants stop at 4
// in the reserved groups until the latecomers join.
func ComputeWithLate(settings models.Settings, countsByProject map[int]int, lateGroups map[int]bool, unappliedLateCount int) (map[int]int, models.QuotaSummary) {
	totalQuota := settings.TotalQuota
	if totalQuota < 0 {
		totalQuota = 0
	}
	if unappliedLateCount < 0 {
		unappliedLateCount = 0
	}

	var baseCap, highCap, allowedHigh int
	if settings.DistributionMode == "fixed_4_5" {
		if totalQuota >= GroupCount*5 {
			baseCap = totalQuota / GroupCount
			rem := totalQuota % GroupCount
			if rem == 0 {
				highCap = baseCap
				allowedHigh = 0
			} else {
				highCap = baseCap + 1
				allowedHigh = rem
			}
		} else if totalQuota >= GroupCount*4 {
			baseCap = 4
			highCap = 5
			allowedHigh = totalQuota - GroupCount*4
		} else {
			baseCap = totalQuota / GroupCount
			rem := totalQuota % GroupCount
			if rem == 0 {
				highCap = baseCap
				allowedHigh = 0
			} else {
				highCap = baseCap + 1
				allowedHigh = rem
			}
		}
	} else {
		// auto_even
		baseCap = totalQuota / GroupCount
		rem := totalQuota % GroupCount
		if rem == 0 {
			highCap = baseCap
			allowedHigh = 0
		} else {
			highCap = baseCap + 1
			allowedHigh = rem
		}
	}

	totalApplied := 0
	filledHigh := 0
	lateGroupCount := 0
	filledNonLateHigh := 0
	for id := 1; id <= GroupCount; id++ {
		c := countsByProject[id]
		isLate := lateGroups != nil && lateGroups[id]
		totalApplied += c
		if isLate {
			lateGroupCount++
		}
		if highCap > baseCap && c >= highCap {
			filledHigh++
			if !isLate {
				filledNonLateHigh++
			}
		}
	}

	remTotal := totalQuota - totalApplied
	if remTotal < 0 {
		remTotal = 0
	}

	remHighSlots := allowedHigh - filledHigh
	if remHighSlots < 0 {
		remHighSlots = 0
	}

	nonLateAllowedHigh := allowedHigh - lateGroupCount - unappliedLateCount
	if nonLateAllowedHigh < 0 {
		nonLateAllowedHigh = 0
	}

	effCaps := make(map[int]int, GroupCount)
	openGroups := 0
	closedGroups := 0

	for id := 1; id <= GroupCount; id++ {
		c := countsByProject[id]
		isLate := lateGroups != nil && lateGroups[id]
		var capVal int

		if remTotal <= 0 {
			// Total global quota reached: no more seats in any group
			capVal = c
		} else if highCap == baseCap {
			capVal = baseCap
			if c > capVal {
				capVal = c
			}
			if c+remTotal < capVal {
				capVal = c + remTotal
			}
		} else if c >= highCap {
			// Already reached the high tier
			capVal = c
		} else if isLate {
			// Any group with a latecomer keeps its 5-member (highCap) guarantee
			capVal = highCap
			if capVal < 5 {
				capVal = 5
			}
			if c+remTotal < capVal {
				capVal = c + remTotal
			}
		} else if filledNonLateHigh < nonLateAllowedHigh {
			// Non-late high-tier slots still available
			capVal = highCap
			if c+remTotal < capVal {
				capVal = c + remTotal
			}
		} else {
			// Non-late high-tier quota filled (or reserved for unapplied latecomers)
			capVal = baseCap
			if c > capVal {
				capVal = c
			}
			if c+remTotal < capVal {
				capVal = c + remTotal
			}
		}

		effCaps[id] = capVal
		if c >= capVal {
			closedGroups++
		} else {
			openGroups++
		}
	}

	var ruleSummary string
	if highCap == baseCap {
		ruleSummary = fmt.Sprintf("총 %d명 · 18개 조 모두 %d명 균등 배정", totalQuota, baseCap)
	} else {
		baseGroups := GroupCount - allowedHigh
		ruleSummary = fmt.Sprintf(
			"총 %d명 · 빨리 차는 %d개 조 %d명 + 나머지 %d개 조 %d명 배부 (5명/상위정원 잔여 %d개 조)",
			totalQuota, allowedHigh, highCap, baseGroups, baseCap, remHighSlots,
		)
		if highCap != 5 {
			ruleSummary = fmt.Sprintf(
				"총 %d명 · 빨리 차는 %d개 조 %d명 + 나머지 %d개 조 %d명 배부 (상위정원 잔여 %d개 조)",
				totalQuota, allowedHigh, highCap, baseGroups, baseCap, remHighSlots,
			)
		}
	}

	summary := models.QuotaSummary{
		GroupCount:         GroupCount,
		TotalQuota:         totalQuota,
		TotalApplied:       totalApplied,
		RemainingTotal:     remTotal,
		BaseCap:            baseCap,
		HighCap:            highCap,
		AllowedHighGroups:  allowedHigh,
		FilledHighGroups:   filledHigh,
		RemainingHighSlots: remHighSlots,
		OpenGroupsCount:    openGroups,
		ClosedGroupsCount:  closedGroups,
		RuleSummary:        ruleSummary,
	}

	return effCaps, summary
}
