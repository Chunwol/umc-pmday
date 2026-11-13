#!/bin/bash
set -euo pipefail

bin=$1
revision=$2
target=/deploy/umc-pmday
backup=/deploy/previous.bin
rel_target=/deploy/release.txt
rel_backup=/deploy/previous.release.txt
candidate=/deploy/.umc-pmday.new
rel_candidate=/deploy/.release.txt.new

install -m 0755 "$bin" "$candidate"
printf '%s\n' "$revision" > "$rel_candidate"

if [[ -f "$target" ]]; then
  cp -p "$target" "$backup.tmp"
  mv -f "$backup.tmp" "$backup"
fi
if [[ -f "$rel_target" ]]; then
  cp -p "$rel_target" "$rel_backup.tmp"
  mv -f "$rel_backup.tmp" "$rel_backup"
fi

mv -f "$candidate" "$target"
mv -f "$rel_candidate" "$rel_target"

for attempt in $(seq 1 40); do
  body=$(curl -fsS --max-time 3 http://app:8080/healthz 2>/dev/null || true)
  if [[ "$body" == "$revision" ]]; then
    printf 'Deployed and verified %s\n' "$revision"
    exit 0
  fi
  sleep 2
done

echo "Deployment health check failed; rolling back..." >&2
if [[ -f "$backup" ]]; then
  cp -p "$backup" "$candidate"
  mv -f "$candidate" "$target"
fi
if [[ -f "$rel_backup" ]]; then
  cp -p "$rel_backup" "$rel_candidate"
  mv -f "$rel_candidate" "$rel_target"
fi
exit 1
