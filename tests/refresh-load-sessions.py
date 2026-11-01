"""Refresh only synthetic users 001-100 for the isolated final load window.

Uses the public Chat login endpoint with normal TLS verification. Credentials and
tokens remain in .private; accounts 101-110 used by client validation are untouched.
"""
import datetime
import importlib.util
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("openim38_seed", ROOT / "tests/seed_accounts.py")
seed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(seed)
if seed.INTERNAL:
    raise RuntimeError("Final load logins must use the public HTTPS endpoint")
state = json.loads((ROOT / ".private/test-accounts.json").read_text(encoding="utf-8"))
expected = [f"upgrade38_u{i:03}" for i in range(1, 101)]
users = {item["userID"]: item for item in state["users"]}
if any(user_id not in users for user_id in expected):
    raise RuntimeError("The exact synthetic load-account range is missing")
sessions = []
for index, user_id in enumerate(expected, start=1):
    user = users[user_id]
    result = seed.call("/chat/account/login", {
        "email": user["email"], "password": seed.md5(state["password"]),
        "platform": 5, "deviceID": "openim38-final-load-" + str(index),
    })
    if result.get("userID") != user_id or not result.get("imToken"):
        raise RuntimeError("Unexpected synthetic identity or missing IM token")
    sessions.append({"userID": user_id, "imToken": result["imToken"], "platformID": 5})
    if index % 20 == 0:
        print(json.dumps({"syntheticLoginsReady": index, "tokensPrinted": False}), flush=True)
output = ROOT / ".private/final-load-sessions.json"
temporary = output.with_suffix(".tmp")
temporary.write_text(json.dumps({
    "baseURL": seed.BASE, "users": sessions,
    "createdAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
}), encoding="utf-8")
temporary.replace(output)
output.chmod(0o600)
print(json.dumps({"refreshed": len(sessions), "untouched": "101-110", "tokensPrinted": False}))
