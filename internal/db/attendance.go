package db

import (
	"context"
	"time"

	"umc-pmday/internal/models"
)

func (s *Store) SaveAttendanceCancellation(ctx context.Context, req models.AttendanceCancellationRequest, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO attendance_cancellations
		(nickname, name, school, phone, reason, submitted_at) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE nickname=VALUES(nickname), reason=VALUES(reason), submitted_at=VALUES(submitted_at)`,
		req.Nickname, req.Name, req.School, req.Phone, req.Reason, now.UTC().Format("2006-01-02 15:04:05.000000"))
	return err
}

func (s *Store) ListAttendanceCancellations(ctx context.Context) ([]models.AttendanceCancellation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, nickname, name, school, phone, reason, submitted_at FROM attendance_cancellations ORDER BY submitted_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.AttendanceCancellation, 0)
	for rows.Next() {
		var c models.AttendanceCancellation
		if err := rows.Scan(&c.ID, &c.Nickname, &c.Name, &c.School, &c.Phone, &c.Reason, &c.SubmittedAt); err != nil {
			return nil, err
		}
		// Stored timestamps are UTC regardless of the MySQL driver's configured location.
		c.SubmittedAt = time.Date(c.SubmittedAt.Year(), c.SubmittedAt.Month(), c.SubmittedAt.Day(), c.SubmittedAt.Hour(), c.SubmittedAt.Minute(), c.SubmittedAt.Second(), c.SubmittedAt.Nanosecond(), time.UTC)
		result = append(result, c)
	}
	return result, rows.Err()
}
