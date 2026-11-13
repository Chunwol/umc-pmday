package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"umc-pmday/internal/capacity"
	"umc-pmday/internal/models"
)

type Store struct {
	DB       *sql.DB
	AdminPIN string
}

func Open(dsn, adminPIN string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db.SetMaxOpenConns(60)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db.Ping: %w", err)
	}

	s := &Store{DB: db, AdminPIN: adminPIN}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.seed(ctx); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id TINYINT NOT NULL PRIMARY KEY,
			total_quota INT NOT NULL DEFAULT 87,
			distribution_mode VARCHAR(16) NOT NULL DEFAULT 'auto_even',
			open_at DATETIME(3) NOT NULL,
			is_paused TINYINT(1) NOT NULL DEFAULT 0,
			updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INT NOT NULL PRIMARY KEY,
			group_no INT NOT NULL,
			title VARCHAR(120) NOT NULL,
			pm_nickname VARCHAR(64) NOT NULL,
			pm_name VARCHAR(64) NOT NULL,
			pm_school VARCHAR(64) NOT NULL,
			original_part VARCHAR(32) NOT NULL,
			thumbnail_url VARCHAR(255) NOT NULL,
			logo_url VARCHAR(255) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS applications (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			arrival_seq BIGINT UNSIGNED NOT NULL,
			project_id INT NOT NULL,
			nickname_key VARCHAR(64) NOT NULL,
			nickname VARCHAR(64) NOT NULL,
			name VARCHAR(64) NOT NULL,
			school VARCHAR(64) NOT NULL,
			phone VARCHAR(32) NOT NULL,
			part VARCHAR(32) NOT NULL,
			clicked_at DATETIME(6) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			UNIQUE KEY uk_nickname_key (nickname_key),
			KEY idx_project_id (project_id),
			KEY idx_arrival_seq (arrival_seq),
			CONSTRAINT fk_app_project FOREIGN KEY (project_id) REFERENCES projects(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS click_logs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			arrival_seq BIGINT UNSIGNED NOT NULL,
			project_id INT NOT NULL,
			project_name VARCHAR(120) NOT NULL,
			nickname VARCHAR(64) NOT NULL,
			name VARCHAR(64) NOT NULL,
			school VARCHAR(64) NOT NULL,
			part VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL,
			message VARCHAR(255) NOT NULL,
			clicked_at DATETIME(6) NOT NULL,
			KEY idx_arrival_seq (arrival_seq),
			KEY idx_clicked_at (clicked_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS challengers (
			id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			nickname VARCHAR(64) NOT NULL,
			name VARCHAR(64) NOT NULL,
			school VARCHAR(64) NOT NULL,
			phone VARCHAR(32) NOT NULL,
			part VARCHAR(32) NOT NULL,
			attendance VARCHAR(32) NOT NULL,
			note VARCHAR(255) NOT NULL DEFAULT '',
			UNIQUE KEY uk_name_phone (name, phone)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS project_assets (
			path VARCHAR(128) NOT NULL PRIMARY KEY,
			mime_type VARCHAR(64) NOT NULL DEFAULT 'image/png',
			data LONGBLOB NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
	}

	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seed(ctx context.Context) error {
	// Default total_quota = 87 (84 on-time + 3 late out of 91 active PE challengers)
	// Default open_at = now - 1m so immediately usable out of the box (admin can set countdown anytime)
	_, err := s.DB.ExecContext(ctx,
		`INSERT IGNORE INTO settings (id, total_quota, distribution_mode, open_at, is_paused) VALUES (1, 87, 'auto_even', ?, 0)`,
		time.Now().Add(-1*time.Minute),
	)
	if err != nil {
		return err
	}

	var projCount int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&projCount); err != nil {
		return err
	}
	if projCount == 0 {
		for _, path := range []string{os.Getenv("SEED_PROJECTS_FILE"), "secrets/seed_projects.json"} {
			if strings.TrimSpace(path) == "" {
				continue
			}
			if b, err := os.ReadFile(path); err == nil {
				var projects []models.Project
				if err := json.Unmarshal(b, &projects); err != nil {
					return fmt.Errorf("parse %s: %w", path, err)
				}
				for _, p := range projects {
					if _, err := s.DB.ExecContext(ctx,
						`INSERT IGNORE INTO projects (id, group_no, title, pm_nickname, pm_name, pm_school, original_part, thumbnail_url, logo_url)
						 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
						p.ID, p.GroupNo, p.Title, p.PMNickname, p.PMName, p.PMSchool, p.OriginalPart, p.ThumbnailURL, p.LogoURL,
					); err != nil {
						return err
					}
				}
				break
			}
		}
	}

	var chalCount int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM challengers`).Scan(&chalCount); err != nil {
		return err
	}
	if chalCount == 0 {
		for _, path := range []string{os.Getenv("SEED_CHALLENGERS_FILE"), "secrets/seed_challengers.json"} {
			if strings.TrimSpace(path) == "" {
				continue
			}
			if b, err := os.ReadFile(path); err == nil {
				var challengers []models.Challenger
				if err := json.Unmarshal(b, &challengers); err != nil {
					return fmt.Errorf("parse %s: %w", path, err)
				}
				for _, c := range challengers {
					if _, err := s.DB.ExecContext(ctx,
						`INSERT IGNORE INTO challengers (nickname, name, school, phone, part, attendance, note)
						 VALUES (?, ?, ?, ?, ?, ?, ?)`,
						c.Nickname, c.Name, c.School, c.Phone, c.Part, c.Attendance, c.Note,
					); err != nil {
						return err
					}
				}
				break
			}
		}
	}

	var assetCount int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_assets`).Scan(&assetCount); err != nil {
		return err
	}
	if assetCount == 0 {
		for _, dir := range []string{os.Getenv("SEED_ASSETS_DIR"), "secrets/images"} {
			if strings.TrimSpace(dir) == "" {
				continue
			}
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				if err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil || d.IsDir() {
						return walkErr
					}
					rel, err := filepath.Rel(dir, p)
					if err != nil {
						return err
					}
					data, err := os.ReadFile(p)
					if err != nil {
						return err
					}
					assetPath := "images/" + filepath.ToSlash(rel)
					_, err = s.DB.ExecContext(ctx,
						`INSERT IGNORE INTO project_assets (path, mime_type, data) VALUES (?, 'image/png', ?)`,
						assetPath, data,
					)
					return err
				}); err != nil {
					return err
				}
				break
			}
		}
	}

	return nil
}

func (s *Store) GetAsset(ctx context.Context, assetPath string) ([]byte, string, error) {
	var data []byte
	var mimeType string
	err := s.DB.QueryRowContext(ctx, `SELECT data, mime_type FROM project_assets WHERE path = ?`, assetPath).Scan(&data, &mimeType)
	return data, mimeType, err
}

func NormalizeNicknameKey(nick string) string {
	s := strings.TrimSpace(nick)
	s = strings.Join(strings.Fields(s), "")
	return strings.ToLower(s)
}

func NormalizeApplicantKey(nick, name string) string {
	return NormalizeNicknameKey(nick) + ":" + NormalizeNicknameKey(name)
}

type rowQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadLateApplicantKeys(ctx context.Context, q rowQueryer) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT nickname, name FROM challengers WHERE attendance = '늦참'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make(map[string]bool)
	for rows.Next() {
		var nick, name string
		if err := rows.Scan(&nick, &name); err != nil {
			return nil, err
		}
		keys[NormalizeApplicantKey(nick, name)] = true
	}
	return keys, rows.Err()
}

func NormalizePhoneDigits(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func MaskPhone(phone string) string {
	d := NormalizePhoneDigits(phone)
	if len(d) >= 4 {
		return "***-" + d[len(d)-4:]
	}
	return "****"
}

func (s *Store) MaxClickSeq(ctx context.Context) (uint64, error) {
	var maxSeq sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT GREATEST(COALESCE((SELECT MAX(arrival_seq) FROM applications), 0), COALESCE((SELECT MAX(arrival_seq) FROM click_logs), 0))`).Scan(&maxSeq); err != nil {
		return 0, err
	}
	if !maxSeq.Valid {
		return 0, nil
	}
	return uint64(maxSeq.Int64), nil
}

func (s *Store) ListChallengers(ctx context.Context) ([]models.Challenger, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, nickname, name, school, phone, part, attendance, note FROM challengers ORDER BY nickname ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Challenger
	for rows.Next() {
		var c models.Challenger
		if err := rows.Scan(&c.ID, &c.Nickname, &c.Name, &c.School, &c.Phone, &c.Part, &c.Attendance, &c.Note); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetState(ctx context.Context) (models.StateResponse, error) {
	var settings models.Settings
	var paused int
	err := s.DB.QueryRowContext(ctx, `SELECT total_quota, distribution_mode, open_at, is_paused FROM settings WHERE id = 1`).
		Scan(&settings.TotalQuota, &settings.DistributionMode, &settings.OpenAt, &paused)
	if err != nil {
		return models.StateResponse{}, err
	}
	settings.IsPaused = paused != 0

	prows, err := s.DB.QueryContext(ctx, `SELECT id, group_no, title, pm_nickname, pm_name, pm_school, original_part, thumbnail_url, logo_url FROM projects ORDER BY group_no ASC`)
	if err != nil {
		return models.StateResponse{}, err
	}
	defer prows.Close()

	var projects []models.Project
	projTitleMap := make(map[int]string, 18)
	for prows.Next() {
		var p models.Project
		if err := prows.Scan(&p.ID, &p.GroupNo, &p.Title, &p.PMNickname, &p.PMName, &p.PMSchool, &p.OriginalPart, &p.ThumbnailURL, &p.LogoURL); err != nil {
			return models.StateResponse{}, err
		}
		projects = append(projects, p)
		projTitleMap[p.ID] = p.Title
	}
	if err := prows.Err(); err != nil {
		return models.StateResponse{}, err
	}

	lateKeys, err := loadLateApplicantKeys(ctx, s.DB)
	if err != nil {
		return models.StateResponse{}, err
	}

	arows, err := s.DB.QueryContext(ctx, `SELECT id, arrival_seq, project_id, nickname, name, school, phone, part, clicked_at FROM applications ORDER BY arrival_seq ASC, id ASC`)
	if err != nil {
		return models.StateResponse{}, err
	}
	defer arows.Close()

	membersByProj := make(map[int][]models.Application, 18)
	countsByProj := make(map[int]int, 18)
	lateGroups := make(map[int]bool, 18)
	appliedLate := make(map[string]bool, len(lateKeys))
	for arows.Next() {
		var a models.Application
		var phoneRaw string
		if err := arows.Scan(&a.ID, &a.ArrivalSeq, &a.ProjectID, &a.Nickname, &a.Name, &a.School, &phoneRaw, &a.Part, &a.ClickedAt); err != nil {
			return models.StateResponse{}, err
		}
		a.PhoneMasked = MaskPhone(phoneRaw)
		a.ClickedText = a.ClickedAt.Format("15:04:05.000000")
		membersByProj[a.ProjectID] = append(membersByProj[a.ProjectID], a)
		countsByProj[a.ProjectID]++
		appKey := NormalizeApplicantKey(a.Nickname, a.Name)
		if lateKeys[appKey] {
			lateGroups[a.ProjectID] = true
			appliedLate[appKey] = true
		}
	}
	if err := arows.Err(); err != nil {
		return models.StateResponse{}, err
	}

	unappliedLateCount := len(lateKeys) - len(appliedLate)
	effCaps, summary := capacity.ComputeWithLate(settings, countsByProj, lateGroups, unappliedLateCount)

	views := make([]models.ProjectView, 0, len(projects))
	for _, p := range projects {
		members := membersByProj[p.ID]
		if members == nil {
			members = []models.Application{}
		}
		curr := countsByProj[p.ID]
		capVal := effCaps[p.ID]
		isFull := curr >= capVal
		label := "모집 중"
		if isFull {
			label = "마감 완료"
		} else if capVal-curr == 1 {
			label = "마감 임박"
		}
		views = append(views, models.ProjectView{
			Project:           p,
			CurrentCount:      curr,
			EffectiveCapacity: capVal,
			IsFull:            isFull,
			StatusLabel:       label,
			Members:           members,
		})
	}

	lrows, err := s.DB.QueryContext(ctx, `SELECT id, arrival_seq, project_id, project_name, nickname, name, school, part, status, message, clicked_at FROM click_logs ORDER BY id DESC LIMIT 120`)
	if err != nil {
		return models.StateResponse{}, err
	}
	defer lrows.Close()

	logs := make([]models.ClickLog, 0, 64)
	for lrows.Next() {
		var l models.ClickLog
		if err := lrows.Scan(&l.ID, &l.ArrivalSeq, &l.ProjectID, &l.ProjectName, &l.Nickname, &l.Name, &l.School, &l.Part, &l.Status, &l.Message, &l.ClickedAt); err != nil {
			return models.StateResponse{}, err
		}
		l.ClickedText = l.ClickedAt.Format("15:04:05.000000")
		logs = append(logs, l)
	}
	if err := lrows.Err(); err != nil {
		return models.StateResponse{}, err
	}

	now := time.Now()
	isOpen := !settings.IsPaused && !now.Before(settings.OpenAt)

	return models.StateResponse{
		ServerTimeMs: now.UnixMilli(),
		OpenAtMs:     settings.OpenAt.UnixMilli(),
		IsOpen:       isOpen,
		Settings:     settings,
		Summary:      summary,
		Projects:     views,
		RecentLogs:   logs,
	}, nil
}

// ApplyAtomic runs inside a single serialized queue worker and uses FOR UPDATE as defense-in-depth.
func (s *Store) ApplyAtomic(ctx context.Context, seq uint64, clickedAt time.Time, req models.ApplyRequest) models.ApplyResponse {
	clickedText := clickedAt.Format("15:04:05.000000")
	nick := strings.TrimSpace(req.Nickname)
	name := strings.TrimSpace(req.Name)
	school := strings.TrimSpace(req.School)
	phone := strings.TrimSpace(req.Phone)
	part := strings.TrimSpace(req.Part)
	nickKey := NormalizeApplicantKey(nick, name)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return models.ApplyResponse{OK: false, Status: "ERROR", Message: "서버 DB 연결 오류가 발생했습니다.", ArrivalSeq: seq, ClickedText: clickedText}
	}
	defer tx.Rollback()

	var settings models.Settings
	var paused int
	if err := tx.QueryRowContext(ctx, `SELECT total_quota, distribution_mode, open_at, is_paused FROM settings WHERE id = 1 FOR UPDATE`).
		Scan(&settings.TotalQuota, &settings.DistributionMode, &settings.OpenAt, &paused); err != nil {
		return models.ApplyResponse{OK: false, Status: "ERROR", Message: "설정을 불러오지 못했습니다.", ArrivalSeq: seq, ClickedText: clickedText}
	}
	settings.IsPaused = paused != 0

	var projTitle string
	if err := tx.QueryRowContext(ctx, `SELECT title FROM projects WHERE id = ? FOR UPDATE`, req.ProjectID).Scan(&projTitle); err != nil {
		return models.ApplyResponse{OK: false, Status: "INVALID", Message: "존재하지 않는 조 번호입니다.", ArrivalSeq: seq, ClickedText: clickedText}
	}

	insertLog := func(status, msg string) {
		_, _ = tx.ExecContext(ctx,
			`INSERT INTO click_logs (arrival_seq, project_id, project_name, nickname, name, school, part, status, message, clicked_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			seq, req.ProjectID, projTitle, nick, name, school, part, status, msg, clickedAt,
		)
	}

	if settings.IsPaused {
		msg := "현재 운영진에 의해 신청이 일시 중지되어 있습니다."
		insertLog("CLOSED", msg)
		_ = tx.Commit()
		state, _ := s.GetState(ctx)
		return models.ApplyResponse{OK: false, Status: "CLOSED", Message: msg, ArrivalSeq: seq, ClickedText: clickedText, State: state}
	}

	if clickedAt.Before(settings.OpenAt) {
		msg := fmt.Sprintf("아직 오픈 전입니다. (%s 오픈)", settings.OpenAt.Format("15:04:05"))
		insertLog("BEFORE_OPEN", msg)
		_ = tx.Commit()
		state, _ := s.GetState(ctx)
		return models.ApplyResponse{OK: false, Status: "BEFORE_OPEN", Message: msg, ArrivalSeq: seq, ClickedText: clickedText, State: state}
	}

	// Check duplicate nickname
	var existingProjID int
	var existingProjTitle string
	err = tx.QueryRowContext(ctx,
		`SELECT a.project_id, p.title FROM applications a JOIN projects p ON p.id = a.project_id WHERE a.nickname_key = ? LIMIT 1`,
		nickKey,
	).Scan(&existingProjID, &existingProjTitle)
	if err == nil {
		msg := fmt.Sprintf("이미 [%d조 %s]에 신청된 본인(닉네임·이름)입니다. (중복 신청 불가)", existingProjID, existingProjTitle)
		insertLog("DUPLICATE", msg)
		_ = tx.Commit()
		state, _ := s.GetState(ctx)
		return models.ApplyResponse{OK: false, Status: "DUPLICATE", Message: msg, ArrivalSeq: seq, ClickedText: clickedText, State: state}
	}
	if err != sql.ErrNoRows {
		return models.ApplyResponse{OK: false, Status: "ERROR", Message: "중복 확인 중 오류가 발생했습니다.", ArrivalSeq: seq, ClickedText: clickedText}
	}

	lateKeys, err := loadLateApplicantKeys(ctx, tx)
	if err != nil {
		return models.ApplyResponse{OK: false, Status: "ERROR", Message: "늦참 대상 확인 중 오류가 발생했습니다.", ArrivalSeq: seq, ClickedText: clickedText}
	}
	isLateApplicant := lateKeys[nickKey]

	// Count current applications across all 18 groups and track latecomer group membership
	crows, err := tx.QueryContext(ctx, `SELECT project_id, nickname_key FROM applications`)
	if err != nil {
		return models.ApplyResponse{OK: false, Status: "ERROR", Message: "정원 집계 중 오류가 발생했습니다.", ArrivalSeq: seq, ClickedText: clickedText}
	}
	countsByProj := make(map[int]int, 18)
	lateGroups := make(map[int]bool, 18)
	appliedLate := make(map[string]bool, len(lateKeys))
	for crows.Next() {
		var pid int
		var appKey string
		if err := crows.Scan(&pid, &appKey); err != nil {
			crows.Close()
			return models.ApplyResponse{OK: false, Status: "ERROR", Message: "정원 스캔 오류", ArrivalSeq: seq, ClickedText: clickedText}
		}
		countsByProj[pid]++
		if lateKeys[appKey] {
			lateGroups[pid] = true
			appliedLate[appKey] = true
		}
	}
	crows.Close()

	unappliedLateCount := len(lateKeys) - len(appliedLate)
	effCaps, summary := capacity.ComputeWithLate(settings, countsByProj, lateGroups, unappliedLateCount)
	currCount := countsByProj[req.ProjectID]
	effCap := effCaps[req.ProjectID]
	if isLateApplicant && effCap < 5 && summary.RemainingTotal > 0 {
		effCap = 5
		if currCount+summary.RemainingTotal < effCap {
			effCap = currCount + summary.RemainingTotal
		}
	}

	if summary.RemainingTotal <= 0 {
		msg := fmt.Sprintf("전체 선착순 정원(%d명)이 모두 마감되었습니다.", settings.TotalQuota)
		insertLog("FULL", msg)
		_ = tx.Commit()
		state, _ := s.GetState(ctx)
		return models.ApplyResponse{OK: false, Status: "FULL", Message: msg, ArrivalSeq: seq, ClickedText: clickedText, State: state}
	}
	if currCount >= effCap {
		msg := fmt.Sprintf("[%d조 %s] 정원(%d명)이 선착순 마감되었습니다.", req.ProjectID, projTitle, effCap)
		insertLog("FULL", msg)
		_ = tx.Commit()
		state, _ := s.GetState(ctx)
		return models.ApplyResponse{OK: false, Status: "FULL", Message: msg, ArrivalSeq: seq, ClickedText: clickedText, State: state}
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO applications (arrival_seq, project_id, nickname_key, nickname, name, school, phone, part, clicked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seq, req.ProjectID, nickKey, nick, name, school, phone, part, clickedAt,
	)
	if err != nil {
		msg := "중복 신청 또는 저장 오류가 발생했습니다."
		insertLog("DUPLICATE", msg)
		_ = tx.Commit()
		state, _ := s.GetState(ctx)
		return models.ApplyResponse{OK: false, Status: "DUPLICATE", Message: msg, ArrivalSeq: seq, ClickedText: clickedText, State: state}
	}
	appID, _ := res.LastInsertId()

	msg := fmt.Sprintf("[%d조 %s] 배정 완료 (%d/%d명)", req.ProjectID, projTitle, currCount+1, effCap)
	insertLog("SUCCESS", msg)

	if err := tx.Commit(); err != nil {
		return models.ApplyResponse{OK: false, Status: "ERROR", Message: "트랜잭션 커밋 실패", ArrivalSeq: seq, ClickedText: clickedText}
	}

	state, _ := s.GetState(ctx)
	return models.ApplyResponse{
		OK:          true,
		Status:      "SUCCESS",
		Message:     msg,
		ArrivalSeq:  seq,
		ClickedText: clickedText,
		Application: &models.Application{
			ID:          appID,
			ArrivalSeq:  seq,
			ProjectID:   req.ProjectID,
			Nickname:    nick,
			Name:        name,
			School:      school,
			PhoneMasked: MaskPhone(phone),
			Part:        part,
			ClickedAt:   clickedAt,
			ClickedText: clickedText,
		},
		State: state,
	}
}

func (s *Store) CancelByUser(ctx context.Context, seq uint64, clickedAt time.Time, req models.CancelRequest) (bool, string) {
	nickKey := NormalizeNicknameKey(req.Nickname)
	phoneInput := NormalizePhoneDigits(req.Phone)
	if nickKey == "" || len(phoneInput) < 4 {
		return false, "닉네임과 전화번호(전체 또는 뒷 4자리)를 입력해 주세요."
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, "DB 연결 오류"
	}
	defer tx.Rollback()

	type cancelCandidate struct {
		appID       int64
		projID      int
		projTitle   string
		nick        string
		name        string
		school      string
		storedPhone string
		part        string
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT a.id, a.project_id, p.title, a.nickname, a.name, a.school, a.phone, a.part
		 FROM applications a JOIN projects p ON p.id = a.project_id
		 WHERE LOWER(REPLACE(a.nickname, ' ', '')) = ? FOR UPDATE`,
		nickKey,
	)
	if err != nil {
		return false, "신청 내역 조회 오류"
	}

	var candidates []cancelCandidate
	for rows.Next() {
		var c cancelCandidate
		if err := rows.Scan(&c.appID, &c.projID, &c.projTitle, &c.nick, &c.name, &c.school, &c.storedPhone, &c.part); err != nil {
			rows.Close()
			return false, "신청 내역 스캔 오류"
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if len(candidates) == 0 {
		return false, "해당 닉네임으로 신청된 내역이 없습니다."
	}

	var matchedCand *cancelCandidate
	for i := range candidates {
		storedDigits := NormalizePhoneDigits(candidates[i].storedPhone)
		if storedDigits == phoneInput || (len(phoneInput) == 4 && strings.HasSuffix(storedDigits, phoneInput)) {
			matchedCand = &candidates[i]
			break
		}
	}
	if matchedCand == nil {
		return false, "전화번호가 일치하지 않아 취소할 수 없습니다."
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM applications WHERE id = ?`, matchedCand.appID); err != nil {
		return false, "신청 취소 처리 중 오류가 발생했습니다."
	}

	msg := fmt.Sprintf("[%d조 %s] 본인 신청 취소", matchedCand.projID, matchedCand.projTitle)
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO click_logs (arrival_seq, project_id, project_name, nickname, name, school, part, status, message, clicked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'CANCELLED', ?, ?)`,
		seq, matchedCand.projID, matchedCand.projTitle, matchedCand.nick, matchedCand.name, matchedCand.school, matchedCand.part, msg, clickedAt,
	)

	if err := tx.Commit(); err != nil {
		return false, "취소 저장 실패"
	}
	return true, msg
}

func (s *Store) UpdateSettings(ctx context.Context, totalQuota int, mode string, openAt time.Time, paused bool) error {
	if mode != "fixed_4_5" {
		mode = "auto_even"
	}
	if totalQuota < 0 {
		totalQuota = 0
	}
	p := 0
	if paused {
		p = 1
	}
	_, err := s.DB.ExecContext(ctx,
		`UPDATE settings SET total_quota = ?, distribution_mode = ?, open_at = ?, is_paused = ? WHERE id = 1`,
		totalQuota, mode, openAt, p,
	)
	return err
}

func (s *Store) AdminRemoveApplication(ctx context.Context, seq uint64, clickedAt time.Time, appID int64) (bool, string) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, "DB 오류"
	}
	defer tx.Rollback()

	var projID int
	var projTitle, nick, name, school, part string
	err = tx.QueryRowContext(ctx,
		`SELECT a.project_id, p.title, a.nickname, a.name, a.school, a.part
		 FROM applications a JOIN projects p ON p.id = a.project_id
		 WHERE a.id = ? FOR UPDATE`, appID,
	).Scan(&projID, &projTitle, &nick, &name, &school, &part)
	if err == sql.ErrNoRows {
		return false, "이미 취소되었거나 존재하지 않는 신청입니다."
	}
	if err != nil {
		return false, "조회 오류"
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM applications WHERE id = ?`, appID); err != nil {
		return false, "삭제 오류"
	}
	msg := fmt.Sprintf("[%d조 %s] 운영진에 의해 배정 취소됨", projID, projTitle)
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO click_logs (arrival_seq, project_id, project_name, nickname, name, school, part, status, message, clicked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'CANCELLED', ?, ?)`,
		seq, projID, projTitle, nick, name, school, part, msg, clickedAt,
	)
	if err := tx.Commit(); err != nil {
		return false, "커밋 실패"
	}
	return true, msg
}

func (s *Store) AdminResetAll(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM applications`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM click_logs`); err != nil {
		return err
	}
	return tx.Commit()
}
