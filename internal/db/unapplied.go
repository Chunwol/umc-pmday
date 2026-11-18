package db

import (
	"context"

	"umc-pmday/internal/models"
)

func buildUnappliedChallengers(challengers []models.Challenger, appliedKeys, cancellationKeys map[string]bool) []models.UnappliedChallenger {
	result := make([]models.UnappliedChallenger, 0)
	for _, challenger := range challengers {
		if challenger.Attendance == "불참" || challenger.Attendance == "운영진" {
			continue
		}
		key := NormalizeApplicantKey(challenger.Nickname, challenger.Name)
		if appliedKeys[key] || cancellationKeys[key] {
			continue
		}
		result = append(result, models.UnappliedChallenger{
			ID:         challenger.ID,
			Nickname:   challenger.Nickname,
			Name:       challenger.Name,
			School:     challenger.School,
			Part:       challenger.Part,
			Attendance: challenger.Attendance,
		})
	}
	return result
}

func (s *Store) attendanceCancellationKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT nickname, name FROM attendance_cancellations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make(map[string]bool)
	for rows.Next() {
		var nickname, name string
		if err := rows.Scan(&nickname, &name); err != nil {
			return nil, err
		}
		keys[NormalizeApplicantKey(nickname, name)] = true
	}
	return keys, rows.Err()
}
