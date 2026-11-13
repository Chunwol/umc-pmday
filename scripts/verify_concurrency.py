import json
import os
import time
import urllib.request
import urllib.error
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

BASE = "http://127.0.0.1:8080"
_pin_file = Path(__file__).resolve().parent.parent / "secrets" / "admin_pin"
ADMIN_PIN = os.environ.get("ADMIN_PIN") or (_pin_file.read_text(encoding="utf-8").strip() if _pin_file.exists() else "changeme")

def req_raw(path, method="GET", body=None):
    data = None
    headers = {}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()

def req_json(path, method="GET", body=None):
    status, raw = req_raw(path, method, body)
    return status, json.loads(raw.decode("utf-8"))

# 1. Check initial state, challengers (91 active PE: Web 60 + Mobile 31), and clean HTML
code, state = req_json("/api/state")
assert code == 200 and len(state["projects"]) == 18, f"Expected 18 projects, got {code}"
code, chals = req_json("/api/challengers")
assert code == 200 and len(chals) == 91, f"Expected 91 active PE challengers, got {len(chals)}"
web_cnt = sum(1 for c in chals if c["part"] == "PE(WEB)")
mob_cnt = sum(1 for c in chals if c["part"] == "PE(Mobile)")
assert web_cnt == 60 and mob_cnt == 31, f"Expected Web=60, Mobile=31, got {web_cnt}, {mob_cnt}"
assert all("(" not in c["nickname"] for c in chals), "Found unexpected nickname suffix"
print(f"[PASS] State OK: 18 projects, {len(chals)} active PE challengers (Web {web_cnt} + Mobile {mob_cnt})")

code, html_bytes = req_raw("/")
html = html_bytes.decode("utf-8")
assert code == 200
for forbidden in ("⚙️", "⚡", "📋", "⏸", "✅", "⚠️", "ℹ️", ADMIN_PIN, "umc1234", "동명이닉", "PM·Design"):
    assert forbidden not in html, f"Forbidden string still in HTML: {forbidden}"
for required in ('id="applyModal"', 'id="cancelModal"', 'id="adminLockView"', 'id="adminTabBar"'):
    assert required in html, f"Missing required element in HTML: {required}"
print("[PASS] HTML clean: no emojis, no leaked password, no PM·Design/duplicate-nick text, apply/cancel modals present")

# 2. Check admin auth & admin-only CSV export
code, _ = req_json("/api/admin/auth", "POST", {"pin": "wrong-pin"})
assert code == 401, f"Expected 401 for wrong PIN, got {code}"
code, auth_ok = req_json("/api/admin/auth", "POST", {"pin": ADMIN_PIN})
assert code == 200 and auth_ok["ok"], f"Expected 200 for valid PIN, got {code}"

code, _ = req_raw("/api/export.csv", "GET")
assert code in (404, 405), f"Expected removed public CSV endpoint, got {code}"
code, _ = req_raw("/api/admin/export.csv", "POST", {"pin": "wrong-pin"})
assert code == 401, f"Expected 401 for unauthorized CSV export, got {code}"
code, csv_bytes = req_raw("/api/admin/export.csv", "POST", {"pin": ADMIN_PIN})
csv_text = csv_bytes.decode("utf-8-sig")
assert code == 200 and "조 번호,프로젝트명" in csv_text, "Admin CSV export failed"
print("[PASS] Admin auth & admin-only CSV export verified")

# 3. Check all 36 downloaded images return 200
for i in range(1, 19):
    for kind in ("thumbnails", "logos"):
        r = urllib.request.urlopen(f"{BASE}/static/images/{kind}/group-{i:02d}.png", timeout=5)
        assert r.status == 200 and int(r.headers.get("Content-Length", "1")) > 0
print("[PASS] All 36 local project images served with HTTP 200")

# 4. Reset & set open time +2 seconds with total_quota = 74 (2 groups of 5, 16 groups of 4)
req_json("/api/admin/reset", "POST", {"pin": ADMIN_PIN})
code, res = req_json("/api/admin/settings", "POST", {
    "pin": ADMIN_PIN,
    "total_quota": 74,
    "distribution_mode": "auto_even",
    "open_offset_sec": 2,
    "is_paused": False,
})
assert code == 200 and res["ok"]

# Attempt before open -> must reject with BEFORE_OPEN
_, early = req_json("/api/apply", "POST", {
    "project_id": 1, "nickname": "조기클릭", "name": "테스트", "school": "동양미래대", "phone": "010-1111-2222", "part": "PE(WEB)"
})
assert not early["ok"] and early["status"] == "BEFORE_OPEN", f"Expected BEFORE_OPEN, got {early}"
print(f"[PASS] Pre-open click blocked cleanly: {early['message']}")

time.sleep(2.3)

# 5. Same-nickname different-person & same-person duplicate guard
c1, teo1 = req_json("/api/apply", "POST", {
    "project_id": 1, "nickname": "중복닉", "name": "사용자A", "school": "숭실대", "phone": "010-1000-1111", "part": "PE(Mobile)"
})
c2, teo2 = req_json("/api/apply", "POST", {
    "project_id": 2, "nickname": "중복닉", "name": "사용자B", "school": "숭실대", "phone": "010-2000-2222", "part": "PE(WEB)"
})
assert c1 == 200 and teo1["ok"] and c2 == 200 and teo2["ok"], f"Same-nick different-person should both succeed: {teo1}, {teo2}"

def dup_worker(i):
    return req_json("/api/apply", "POST", {
        "project_id": (i % 3) + 1, "nickname": "중복닉", "name": "사용자A", "school": "숭실대", "phone": "010-1000-1111", "part": "PE(Mobile)"
    })[1]

with ThreadPoolExecutor(max_workers=5) as ex:
    dup_results = list(ex.map(dup_worker, range(5)))
assert all(r["status"] == "DUPLICATE" for r in dup_results), f"Expected all DUPLICATE for 사용자A: {dup_results}"

_, cancel_teo2 = req_json("/api/cancel", "POST", {"nickname": "중복닉", "phone": "2222"})
assert cancel_teo2["ok"], f"Cancel by phone suffix failed: {cancel_teo2}"
print("[PASS] Same-nick different-person OK (사용자A & 사용자B both applied, same-person blocked, phone-tail cancel isolated)")

# 6. Reset and run 100-user high-concurrency burst for total_quota=77 (with 3 unapplied latecomers -> regulars fill 2 groups at 5 + 16 groups at 4 = 74)
req_json("/api/admin/reset", "POST", {"pin": ADMIN_PIN})
req_json("/api/admin/settings", "POST", {
    "pin": ADMIN_PIN,
    "total_quota": 77,
    "distribution_mode": "auto_even",
    "open_offset_sec": -1,
    "is_paused": False,
})

jobs = []
for idx in range(100):
    gid = (idx % 18) + 1
    jobs.append({
        "project_id": gid,
        "nickname": f"유저{idx+1:03d}",
        "name": f"이름{idx+1:03d}",
        "school": "동양미래대",
        "phone": f"010-9000-{idx+1:04d}",
        "part": "PE(WEB)" if idx % 2 == 0 else "PE(Mobile)",
    })

def burst_worker(payload):
    return req_json("/api/apply", "POST", payload)

with ThreadPoolExecutor(max_workers=50) as ex:
    burst_res = list(ex.map(burst_worker, jobs))

assert all(code == 200 for code, _ in burst_res), "Some requests failed with non-200 HTTP status!"
success_cnt = sum(1 for _, r in burst_res if r["ok"])
_, final_state = req_json("/api/state")
counts = [p["current_count"] for p in final_state["projects"]]
five_groups = sum(1 for c in counts if c == 5)
four_groups = sum(1 for c in counts if c == 4)
print(f"[PASS] 100-concurrent burst OK: total_applied={final_state['summary']['total_applied']}, 5-member groups={five_groups}, 4-member groups={four_groups}, counts={counts}")
assert success_cnt == 74 and final_state["summary"]["total_applied"] == 74
assert five_groups == 2 and four_groups == 16

# 7. Test total_quota=87 latecomer 5-member guarantee (both regular-first and late-first)
req_json("/api/admin/reset", "POST", {"pin": ADMIN_PIN})
req_json("/api/admin/settings", "POST", {
    "pin": ADMIN_PIN,
    "total_quota": 87,
    "distribution_mode": "auto_even",
    "open_offset_sec": -1,
    "is_paused": False,
})

# 7A) 84 on-time applicants apply across groups 1..18 -> must fill 12 groups of 5 (60) + 6 groups of 4 (24) = 84
for round_i in range(5):
    for gid in range(1, 19):
        idx = round_i * 18 + gid
        if idx > 84:
            break
        req_json("/api/apply", "POST", {
            "project_id": gid, "nickname": f"정시{idx:02d}", "name": f"정시{idx:02d}",
            "school": "동양미래대", "phone": f"010-8000-{idx:04d}", "part": "PE(WEB)",
        })

_, mid_state = req_json("/api/state")
assert mid_state["summary"]["total_applied"] == 84, f"Expected 84 regular applied, got {mid_state['summary']['total_applied']}"
# An 85th regular applicant must be blocked while 3 latecomers remain unapplied
_, reg85 = req_json("/api/apply", "POST", {
    "project_id": 18, "nickname": "정시85", "name": "정시85",
    "school": "동양미래대", "phone": "010-8000-0085", "part": "PE(WEB)",
})
assert not reg85["ok"] and reg85["status"] == "FULL", f"85th regular applicant should not steal latecomer slot: {reg85}"

# Now the 3 latecomers join groups 16, 17, 18 (currently 4 members each) -> each must become 5 members!
late_chals = [c for c in chals if c["attendance"] == "늦참"]
assert len(late_chals) == 3, f"Expected 3 latecomers, got {len(late_chals)}"
latecomers = [
    (16 + i, c["nickname"], c["name"], c["school"], c["phone"], c["part"])
    for i, c in enumerate(late_chals)
]
for gid, nick, nm, sch, ph, pt in latecomers:
    _, lr = req_json("/api/apply", "POST", {
        "project_id": gid, "nickname": nick, "name": nm, "school": sch, "phone": ph, "part": pt,
    })
    assert lr["ok"], f"Latecomer {nick} failed to enter 4-member group {gid}: {lr}"

_, after_late = req_json("/api/state")
counts87 = [p["current_count"] for p in after_late["projects"]]
assert after_late["summary"]["total_applied"] == 87
assert counts87[15] == 5 and counts87[16] == 5 and counts87[17] == 5
assert sum(1 for c in counts87 if c == 5) == 15 and sum(1 for c in counts87 if c == 4) == 3

# 7B) Latecomers apply FIRST into groups 16, 17, 18; then 84 regulars apply across 1..18 -> groups 16,17,18 must still reach 5!
req_json("/api/admin/reset", "POST", {"pin": ADMIN_PIN})
for gid, nick, nm, sch, ph, pt in latecomers:
    _, lr = req_json("/api/apply", "POST", {
        "project_id": gid, "nickname": nick, "name": nm, "school": sch, "phone": ph, "part": pt,
    })
    assert lr["ok"]
for ridx in range(1, 88):
    # Try groups 1..18 round-robin
    for off in range(18):
        gid = ((ridx + off - 1) % 18) + 1
        _, rr = req_json("/api/apply", "POST", {
            "project_id": gid, "nickname": f"선착{ridx:02d}", "name": f"선착{ridx:02d}",
            "school": "숭실대", "phone": f"010-7000-{ridx:04d}", "part": "PE(WEB)",
        })
        if rr["ok"]:
            break
_, early_late = req_json("/api/state")
counts_early = [p["current_count"] for p in early_late["projects"]]
assert early_late["summary"]["total_applied"] == 87
assert counts_early[15] == 5 and counts_early[16] == 5 and counts_early[17] == 5, f"Latecomer groups must all be 5: {counts_early}"
print(f"[PASS] Total 87 latecomer 5-member guarantee OK (both late-arrival and early-arrival -> counts={counts_early})")

# 8. Restore clean production defaults (total_quota=87, open now, 0 test applications)
req_json("/api/admin/reset", "POST", {"pin": ADMIN_PIN})
req_json("/api/admin/settings", "POST", {
    "pin": ADMIN_PIN,
    "total_quota": 87,
    "distribution_mode": "auto_even",
    "open_offset_sec": -60,
    "is_paused": False,
})
print("[PASS] Database reset to clean production state (total_quota=87, open=now)")
