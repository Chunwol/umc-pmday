package capacity

import (
	"fmt"
	"umc-pmday/internal/models"
)

const GroupCount = 18

// Compute computes per-group effective capacities and summary for the 18 groups.
func Compute(settings models.Settings, countsByProject map[int]int) (map[int]int, models.QuotaSummary) {
	return compute(settings, countsByProject)
}

func compute(settings models.Settings, countsByProject map[int]int) (map[int]int, models.QuotaSummary) {
	totalQuota := settings.TotalQuota
	if totalQuota < 0 {
		totalQuota = 0
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
	for id := 1; id <= GroupCount; id++ {
		c := countsByProject[id]
		totalApplied += c
		if highCap > baseCap && c >= highCap {
			filledHigh++
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

	effCaps := make(map[int]int, GroupCount)
	openGroups := 0
	closedGroups := 0

	for id := 1; id <= GroupCount; id++ {
		c := countsByProject[id]
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
		} else if filledHigh < allowedHigh {
			// High-tier slots are shared by all applicants and allocated by arrival order.
			capVal = highCap
			if c+remTotal < capVal {
				capVal = c + remTotal
			}
		} else {
			// All high-tier slots are filled.
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
