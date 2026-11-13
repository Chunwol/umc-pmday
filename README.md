# UMC PM Day 선착순 조 배정 웹 애플리케이션 (Go + MySQL)

18개 프로젝트(1조~18조)에 PE 개발자들이 **오픈 시간 카운트다운** 이후 **선착순으로 신청**하고, **누가 먼저 눌렀는지(µs 순번 로그)** 투명하게 확인하며, **전체 PE 참석자 수에 따라 빨리 차는 순서대로 정원을 자동 배분(5명~4명 또는 균등 분배)**하는 통합 Go + MySQL 풀스택 애플리케이션입니다.

## 실행 방법

1. 전용 MySQL 8.0 (포트 3307) 또는 기존 MySQL 실행:
   - 기본 DSN: `root@tcp(127.0.0.1:3307)/umc_pmday?parseTime=true&loc=Local&charset=utf8mb4`
   - 환경변수 `MYSQL_DSN`으로 커스텀 MySQL 연결 가능
   - `PORT` 환경변수로 웹 서버 포트 변경 가능 (기본 `:8080`)
2. 실행 스크립트:
   - `.\scripts\start.ps1` (MySQL 3307 자동 확인/기동 + Go 서버 실행)
   - 또는 `go run ./cmd/server`
3. 접속: `http://localhost:8080`
   - 운영진 비밀번호: `secrets/admin_pin` (`ADMIN_PIN_FILE`) 또는 환경변수 `ADMIN_PIN` (미설정 시 `changeme`)
