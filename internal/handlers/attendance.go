package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"umc-pmday/internal/db"
	"umc-pmday/internal/models"
)

var attendanceCancellationDeadline = time.Date(2026, 11, 21, 0, 0, 0, 0, time.FixedZone("KST", 9*60*60))

func validateAttendanceCancellation(req models.AttendanceCancellationRequest, now time.Time) string {
	if !now.Before(attendanceCancellationDeadline) {
		return "PM Day 참석 취소 신청은 11월 20일(금) 23:59까지 가능합니다. 접수가 마감되었습니다."
	}
	for _, field := range []string{req.Nickname, req.Name, req.School} {
		if strings.TrimSpace(field) == "" || utf8.RuneCountInString(field) > 64 {
			return "닉네임, 이름, 학교를 각각 64자 이내로 입력해 주세요."
		}
	}
	phone := db.NormalizePhoneDigits(req.Phone)
	if len(phone) < 10 || len(phone) > 11 {
		return "본인 확인을 위해 전체 전화번호를 입력해 주세요."
	}
	if strings.TrimSpace(req.Reason) == "" || utf8.RuneCountInString(req.Reason) > 2000 {
		return "필수 참석 행사이므로 취소 사유를 반드시 입력해 주세요. (최대 2,000자)"
	}
	return ""
}

func (s *Server) handleAttendanceCancellation(w http.ResponseWriter, r *http.Request) {
	var req models.AttendanceCancellationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Name = strings.TrimSpace(req.Name)
	req.School = strings.TrimSpace(req.School)
	req.Phone = db.NormalizePhoneDigits(req.Phone)
	req.Reason = strings.TrimSpace(req.Reason)
	now := time.Now()
	if msg := validateAttendanceCancellation(req, now); msg != "" {
		code := http.StatusBadRequest
		if !now.Before(attendanceCancellationDeadline) {
			code = http.StatusForbidden
		}
		writeJSON(w, code, map[string]any{"ok": false, "message": msg})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.Store.SaveAttendanceCancellation(ctx, req, now); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "message": "저장하지 못했습니다. 잠시 후 다시 시도해 주세요."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "참석 취소 신청이 접수되었습니다. 운영진 검토 후 안내됩니다. 참석 취소가 확정된 것은 아닙니다. 동일 정보로 재제출하면 사유가 수정됩니다."})
}

func (s *Server) handleAdminAttendanceCancellations(w http.ResponseWriter, r *http.Request) {
	var req models.AdminActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	if !s.checkAdminPIN(req.Pin) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "message": "운영자 인증이 필요합니다."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	list, err := s.Store.ListAttendanceCancellations(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "message": "신청 목록을 불러오지 못했습니다."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "requests": list})
}
