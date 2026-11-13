package handlers

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"umc-pmday/internal/db"
	"umc-pmday/internal/models"
	"umc-pmday/internal/queue"
)

type Server struct {
	Store     *db.Store
	Coord     *queue.Coordinator
	Hub       *queue.Hub
	Templates *template.Template
	StaticFS  fs.FS
}

func NewServer(store *db.Store, coord *queue.Coordinator, hub *queue.Hub, webFS fs.FS) (*Server, error) {
	tmpl, err := template.ParseFS(webFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{
		Store:     store,
		Coord:     coord,
		Hub:       hub,
		Templates: tmpl,
	}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /static/", s.handleStatic)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/challengers", s.handleChallengers)
	mux.HandleFunc("GET /api/stream", s.handleStream)
	mux.HandleFunc("POST /api/apply", s.handleApply)
	mux.HandleFunc("POST /api/cancel", s.handleCancel)
	mux.HandleFunc("POST /api/admin/auth", s.handleAdminAuth)
	mux.HandleFunc("POST /api/admin/settings", s.handleAdminSettings)
	mux.HandleFunc("POST /api/admin/remove", s.handleAdminRemove)
	mux.HandleFunc("POST /api/admin/reset", s.handleAdminReset)
	mux.HandleFunc("POST /api/admin/export.csv", s.handleAdminExportCSV)
	return mux
}

func (s *Server) checkAdminPIN(pin string) bool {
	a := strings.TrimSpace(pin)
	b := s.Store.AdminPIN
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.Store.DB.PingContext(ctx); err != nil {
		http.Error(w, "db unhealthy", http.StatusServiceUnavailable)
		return
	}
	rel := strings.TrimSpace(os.Getenv("APP_RELEASE"))
	if rel == "" {
		for _, path := range []string{os.Getenv("RELEASE_FILE"), "/app/release.txt", "release.txt"} {
			if strings.TrimSpace(path) == "" {
				continue
			}
			if b, err := os.ReadFile(path); err == nil {
				if v := strings.TrimSpace(string(b)); v != "" {
					rel = v
					break
				}
			}
		}
	}
	if rel == "" {
		rel = "ok"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(rel + "\n"))
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	assetPath := strings.TrimPrefix(r.URL.Path, "/static/")
	if assetPath == "" || strings.Contains(assetPath, "..") {
		http.NotFound(w, r)
		return
	}
	data, mimeType, err := s.Store.GetAsset(r.Context(), assetPath)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "asset lookup failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	state, err := s.Store.GetState(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleChallengers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListChallengers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	var req models.ApplyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, models.ApplyResponse{OK: false, Status: "INVALID", Message: "잘못된 요청 형식입니다."})
		return
	}

	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Name = strings.TrimSpace(req.Name)
	req.School = strings.TrimSpace(req.School)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Part = strings.TrimSpace(req.Part)

	if req.ProjectID < 1 || req.ProjectID > 18 {
		writeJSON(w, http.StatusBadRequest, models.ApplyResponse{OK: false, Status: "INVALID", Message: "올바른 조(1~18조)를 선택해 주세요."})
		return
	}
	if req.Nickname == "" || req.Name == "" || req.School == "" || req.Phone == "" || req.Part == "" {
		writeJSON(w, http.StatusBadRequest, models.ApplyResponse{OK: false, Status: "INVALID", Message: "닉네임, 이름, 학교, 전화번호, 파트를 모두 입력해 주세요."})
		return
	}
	if len(db.NormalizePhoneDigits(req.Phone)) < 10 {
		writeJSON(w, http.StatusBadRequest, models.ApplyResponse{OK: false, Status: "INVALID", Message: "올바른 전화번호(예: 010-1234-5678)를 입력해 주세요."})
		return
	}
	if req.Part != "PE(WEB)" && req.Part != "PE(Mobile)" {
		writeJSON(w, http.StatusBadRequest, models.ApplyResponse{OK: false, Status: "INVALID", Message: "파트는 PE(WEB) 또는 PE(Mobile)만 선택할 수 있습니다."})
		return
	}

	resp := s.Coord.EnqueueApply(r.Context(), req)
	code := http.StatusOK
	if !resp.OK && resp.Status == "BUSY" {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, resp)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	var req models.CancelRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	res := s.Coord.EnqueueCancel(r.Context(), req)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAdminAuth(w http.ResponseWriter, r *http.Request) {
	var req models.AdminActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	if !s.checkAdminPIN(req.Pin) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "message": "비밀번호가 올바르지 않습니다."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	var req models.AdminSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	if !s.checkAdminPIN(req.Pin) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "message": "비밀번호가 올바르지 않습니다."})
		return
	}

	var openAt time.Time
	if req.OpenOffsetSec != nil {
		openAt = time.Now().Add(time.Duration(*req.OpenOffsetSec) * time.Second)
	} else if strings.TrimSpace(req.OpenAtISO) != "" {
		// Support datetime-local ("2006-01-02T15:04" or "2006-01-02T15:04:05") in local timezone as well as RFC3339
		raw := strings.TrimSpace(req.OpenAtISO)
		var parsed time.Time
		var err error
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
			if layout == time.RFC3339 {
				parsed, err = time.Parse(layout, raw)
			} else {
				parsed, err = time.ParseInLocation(layout, raw, time.Local)
			}
			if err == nil {
				break
			}
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "오픈 시각 형식이 올바르지 않습니다."})
			return
		}
		openAt = parsed
	} else {
		curr, err := s.Store.GetState(r.Context())
		if err != nil {
			openAt = time.Now()
		} else {
			openAt = curr.Settings.OpenAt
		}
	}

	res := s.Coord.EnqueueAdminSettings(r.Context(), req.TotalQuota, req.DistributionMode, openAt, req.IsPaused)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAdminRemove(w http.ResponseWriter, r *http.Request) {
	var req models.AdminActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	if !s.checkAdminPIN(req.Pin) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "message": "비밀번호가 올바르지 않습니다."})
		return
	}
	res := s.Coord.EnqueueAdminRemove(r.Context(), req.ApplicationID)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	var req models.AdminActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	if !s.checkAdminPIN(req.Pin) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "message": "비밀번호가 올바르지 않습니다."})
		return
	}
	res := s.Coord.EnqueueAdminReset(r.Context())
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.Hub.Subscribe()
	defer s.Hub.Unsubscribe(ch)

	// Send initial snapshot immediately
	if state, err := s.Store.GetState(r.Context()); err == nil {
		if b, err := json.Marshal(state); err == nil {
			_, _ = fmt.Fprintf(w, "event: state\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "event: state\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": ping %d\n\n", time.Now().UnixMilli())
			flusher.Flush()
		}
	}
}

func (s *Server) handleAdminExportCSV(w http.ResponseWriter, r *http.Request) {
	var req models.AdminActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "잘못된 요청입니다."})
		return
	}
	if !s.checkAdminPIN(req.Pin) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "message": "비밀번호가 올바르지 않습니다."})
		return
	}
	state, err := s.Store.GetState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="umc-pmday-groups.csv"`)
	// UTF-8 BOM for Excel compatibility
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"조 번호", "프로젝트명", "PM(닉네임/이름)", "PM 학교", "현재 인원", "유효 정원", "순번", "개발자 닉네임", "이름", "학교", "파트", "클릭 시각(µs)"})
	for _, p := range state.Projects {
		pmLabel := fmt.Sprintf("%s/%s", p.PMNickname, p.PMName)
		if len(p.Members) == 0 {
			_ = cw.Write([]string{
				strconv.Itoa(p.GroupNo) + "조",
				p.Title,
				pmLabel,
				p.PMSchool,
				strconv.Itoa(p.CurrentCount),
				strconv.Itoa(p.EffectiveCapacity),
				"-", "-", "-", "-", "-", "-",
			})
			continue
		}
		for _, m := range p.Members {
			_ = cw.Write([]string{
				strconv.Itoa(p.GroupNo) + "조",
				p.Title,
				pmLabel,
				p.PMSchool,
				strconv.Itoa(p.CurrentCount),
				strconv.Itoa(p.EffectiveCapacity),
				"#" + strconv.FormatUint(m.ArrivalSeq, 10),
				m.Nickname,
				m.Name,
				m.School,
				m.Part,
				m.ClickedAt.Format("2006-01-02 15:04:05.000000"),
			})
		}
	}
	cw.Flush()
}
