package models

import "time"

// Settings controls global quota, distribution algorithm, and scheduled open time.
type Settings struct {
	TotalQuota       int       `json:"total_quota"`
	DistributionMode string    `json:"distribution_mode"` // "auto_even" | "fixed_4_5"
	OpenAt           time.Time `json:"open_at"`
	IsPaused         bool      `json:"is_paused"`
}

// Project represents one of the 18 PM Day groups.
type Project struct {
	ID           int    `json:"id"`
	GroupNo      int    `json:"group_no"`
	Title        string `json:"title"`
	PMNickname   string `json:"pm_nickname"`
	PMName       string `json:"pm_name"`
	PMSchool     string `json:"pm_school"`
	OriginalPart string `json:"original_part"`
	ThumbnailURL string `json:"thumbnail_url"`
	LogoURL      string `json:"logo_url"`
}

// Application is a confirmed first-come-first-served seat in a project group.
type Application struct {
	ID          int64     `json:"id"`
	ArrivalSeq  uint64    `json:"arrival_seq"`
	ProjectID   int       `json:"project_id"`
	NicknameKey string    `json:"-"`
	Nickname    string    `json:"nickname"`
	Name        string    `json:"name"`
	School      string    `json:"school"`
	Phone       string    `json:"-"`
	PhoneMasked string    `json:"phone_masked"`
	Part        string    `json:"part"`
	ClickedAt   time.Time `json:"clicked_at"`
	ClickedText string    `json:"clicked_text"`
}

// Challenger is a pre-registered PE developer from the PM Day attendance survey.
type Challenger struct {
	ID         int    `json:"id"`
	Nickname   string `json:"nickname"`
	Name       string `json:"name"`
	School     string `json:"school"`
	Phone      string `json:"phone"`
	Part       string `json:"part"`
	Attendance string `json:"attendance"` // 참석, 늦참, 불참
	Note       string `json:"note"`
}

// ClickLog records every apply/cancel attempt with microsecond timestamp & arrival sequence.
type ClickLog struct {
	ID          int64     `json:"id"`
	ArrivalSeq  uint64    `json:"arrival_seq"`
	ProjectID   int       `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Nickname    string    `json:"nickname"`
	Name        string    `json:"name"`
	School      string    `json:"school"`
	Part        string    `json:"part"`
	Status      string    `json:"status"` // SUCCESS, FULL, DUPLICATE, CLOSED, CANCELLED
	Message     string    `json:"message"`
	ClickedAt   time.Time `json:"clicked_at"`
	ClickedText string    `json:"clicked_text"`
}

// ProjectView enriches Project with live counts, dynamic capacity, and enrolled members.
type ProjectView struct {
	Project
	CurrentCount      int           `json:"current_count"`
	EffectiveCapacity int           `json:"effective_capacity"`
	IsFull            bool          `json:"is_full"`
	StatusLabel       string        `json:"status_label"` // "모집 중" | "마감 임박" | "마감 완료"
	Members           []Application `json:"members"`
}

// QuotaSummary summarizes how total_quota is split across 18 groups.
type QuotaSummary struct {
	GroupCount         int    `json:"group_count"`
	TotalQuota         int    `json:"total_quota"`
	TotalApplied       int    `json:"total_applied"`
	RemainingTotal     int    `json:"remaining_total"`
	BaseCap            int    `json:"base_cap"`
	HighCap            int    `json:"high_cap"`
	AllowedHighGroups  int    `json:"allowed_high_groups"`
	FilledHighGroups   int    `json:"filled_high_groups"`
	RemainingHighSlots int    `json:"remaining_high_slots"`
	OpenGroupsCount    int    `json:"open_groups_count"`
	ClosedGroupsCount  int    `json:"closed_groups_count"`
	RuleSummary        string `json:"rule_summary"`
}

// StateResponse is the full real-time snapshot sent to clients and over SSE.
type StateResponse struct {
	ServerTimeMs int64         `json:"server_time_ms"`
	OpenAtMs     int64         `json:"open_at_ms"`
	IsOpen       bool          `json:"is_open"`
	Settings     Settings      `json:"settings"`
	Summary      QuotaSummary  `json:"summary"`
	Projects     []ProjectView `json:"projects"`
	RecentLogs   []ClickLog    `json:"recent_logs"`
}

// ApplyRequest represents a user's registration click.
type ApplyRequest struct {
	ProjectID int    `json:"project_id"`
	Nickname  string `json:"nickname"`
	Name      string `json:"name"`
	School    string `json:"school"`
	Phone     string `json:"phone"`
	Part      string `json:"part"`
}

// ApplyResponse is returned synchronously after serialized queue processing.
type ApplyResponse struct {
	OK          bool          `json:"ok"`
	Status      string        `json:"status"`
	Message     string        `json:"message"`
	ArrivalSeq  uint64        `json:"arrival_seq"`
	ClickedText string        `json:"clicked_text"`
	Application *Application  `json:"application,omitempty"`
	State       StateResponse `json:"state"`
}

// CancelRequest verifies identity before removing an application.
type CancelRequest struct {
	Nickname string `json:"nickname"`
	Phone    string `json:"phone"`
}

// AdminSettingsRequest updates global quota, mode, open time, or pause state.
type AdminSettingsRequest struct {
	Pin              string `json:"pin"`
	TotalQuota       int    `json:"total_quota"`
	DistributionMode string `json:"distribution_mode"`
	OpenAtISO        string `json:"open_at_iso"`
	OpenOffsetSec    *int   `json:"open_offset_sec,omitempty"`
	IsPaused         bool   `json:"is_paused"`
}

// AdminActionRequest handles admin reset or removal.
type AdminActionRequest struct {
	Pin           string `json:"pin"`
	ApplicationID int64  `json:"application_id,omitempty"`
}
