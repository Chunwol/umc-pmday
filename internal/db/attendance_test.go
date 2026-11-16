package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"umc-pmday/internal/models"
)

// Uses a newly created, disposable schema; never touches the service database.
func TestAttendanceCancellationPersistence(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set")
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	name := fmt.Sprintf("umc_attendance_test_%d", time.Now().UnixNano())
	if _, err := conn.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec("DROP DATABASE " + name)
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	cfg.ParseTime = true
	cfg.Loc = time.Local
	testDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()
	s := &Store{DB: testDB}
	ctx := context.Background()
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	req := models.AttendanceCancellationRequest{Nickname: "test", Name: "Example", School: "Test school", Phone: "01000000000", Reason: "<script>example</script>\nRequired class"}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.SaveAttendanceCancellation(ctx, req, now); err != nil {
		t.Fatal(err)
	}
	req.Reason = "Updated reason\nLine two"
	if err := s.SaveAttendanceCancellation(ctx, req, now); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListAttendanceCancellations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Reason != req.Reason || !list[0].SubmittedAt.Equal(now) {
		t.Fatalf("unexpected saved result: %+v", list)
	}
	var count int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count); err != nil || count != 0 {
		t.Fatalf("group assignments changed: count %d, err %v", count, err)
	}
}
