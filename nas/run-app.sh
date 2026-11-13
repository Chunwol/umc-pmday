#!/bin/sh
set -eu

export MYSQL_DSN="umc:$(tr -d '\r\n' < /run/secrets/db_password)@tcp(db:3306)/umc_pmday?parseTime=true&loc=Asia%2FSeoul&charset=utf8mb4"

while [ ! -x /deploy/umc-pmday ]; do
  echo "[run-app] waiting for /deploy/umc-pmday..."
  sleep 2
done

while true; do
  CUR_REL=""
  [ -f /deploy/release.txt ] && CUR_REL=$(tr -d '\r\n' < /deploy/release.txt)
  cp -f /deploy/umc-pmday /tmp/umc-pmday.bin
  chmod +x /tmp/umc-pmday.bin

  echo "[run-app] starting umc-pmday (release=${CUR_REL:-init})"
  /tmp/umc-pmday.bin &
  APP_PID=$!

  while kill -0 "$APP_PID" 2>/dev/null; do
    NEW_REL=""
    [ -f /deploy/release.txt ] && NEW_REL=$(tr -d '\r\n' < /deploy/release.txt)
    if [ "$NEW_REL" != "$CUR_REL" ]; then
      echo "[run-app] release changed ($CUR_REL -> $NEW_REL), stopping pid $APP_PID..."
      kill -TERM "$APP_PID" 2>/dev/null || true
      break
    fi
    sleep 1
  done

  wait "$APP_PID" || true
  sleep 1
done
