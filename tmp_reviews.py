import json
import subprocess

out = subprocess.check_output(
    ["gh", "api", "repos/inclunet/assistente/pulls/563/reviews", "--paginate"],
    text=True,
    encoding="utf-8",
)
for r in json.loads(out):
    body = r.get("body") or ""
    if not body.strip():
        continue
    print("=" * 70)
    print(r["user"]["login"], r["state"], r.get("submitted_at"))
    print(body)
    print()
