#!/usr/bin/env python3
"""Sign a PDF with Liro Bridge, from Python, with nothing but the standard library.

    python sign.py ugovor.pdf

Reads the pairing from LIRO_APP_ID and LIRO_SECRET when they are set, and
pairs otherwise — the agent shows six digits on the person's screen and
this asks for them.

Written against docs/PROTOCOL.md. The four mistakes a first integration
makes are avoided here by construction, and each is marked below:

  1. a wrong field name          -> every name comes from §5.2's example
  2. origin missing from confirm -> ONE origin variable, used in both calls
  3. a body altered between being hashed and being sent -> the bytes are
     built once and both hashed and sent
  4. a Content-Type on a request with no body -> only set when there is one
"""

import base64
import hashlib
import hmac
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid

# A certificate's display name is a Serbian name — "Zoran Milovanović",
# "ВЕЉКО СТАНОЈЕВИЋ" — which is the normal case here, not an edge one.
# A Windows console runs on a legacy code page (cp1252 on this machine,
# cp852 or cp1251 on others) and printing one to it raises
# UnicodeEncodeError. Measured: without this line the example dies on
# the certificate listing, every time, against a real store.
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

# --- README ---
BASE = "http://127.0.0.1:%d" % json.load(
    open(os.path.join(os.environ["LOCALAPPDATA"], "Liro", "bridge.json"))
)["port"]                                     # read the file; never scan ports


def call(method, path, body=None, app_id=None, secret=None):
    raw = b"" if body is None else json.dumps(body).encode("utf-8")   # hashed AND sent
    headers = {}
    if raw:
        headers["Content-Type"] = "application/json"   # only when there is a body
    if secret:
        ts, nonce = str(int(time.time())), uuid.uuid4().hex
        canon = "\n".join([method, path, ts, nonce, hashlib.sha256(raw).hexdigest()])
        headers.update({
            "X-Liro-App-Id": app_id, "X-Liro-Timestamp": ts, "X-Liro-Nonce": nonce,
            "X-Liro-Signature": hmac.new(base64.b64decode(secret),
                                         canon.encode("utf-8"), hashlib.sha256).hexdigest(),
        })
    req = urllib.request.Request(BASE + path, data=raw or None, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, json.loads(r.read() or b"null")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"null")     # {"code": "...", "details": {...}}
# --- /README ---


def pair(origin, name):
    """The two calls, with the same origin in both — the field that costs an afternoon."""
    status, req = call("POST", "/v2/pair/request", {"applicationName": name, "origin": origin})
    if status != 200:
        raise SystemExit("pair/request: %s" % req)
    print("Liro Bridge is showing six digits on the screen.")
    code = input("Code: ").strip()
    status, ok = call(
        "POST",
        "/v2/pair/confirm",
        {"requestId": req["requestId"], "code": code, "origin": origin},  # the same origin
    )
    if status != 200:
        raise SystemExit("pair/confirm: %s" % ok)
    print("Keep these; the secret is returned once and never again:")
    print('  set LIRO_APP_ID=%s' % ok["appId"])
    print('  set LIRO_SECRET=%s' % ok["deviceSecret"])
    return ok["appId"], ok["deviceSecret"]


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: python sign.py <document.pdf>")
    path = sys.argv[1]

    status, health = call("GET", "/v2/health")
    print("agent %s, protocol %s" % (health["agentVersion"], health["protocolVersion"]))

    origin = os.environ.get("LIRO_ORIGIN", "local")
    app_id, secret = os.environ.get("LIRO_APP_ID"), os.environ.get("LIRO_SECRET")
    if not app_id or not secret:
        app_id, secret = pair(origin, "Python primer")

    status, certs = call("GET", "/v2/certificates", None, app_id, secret)
    if status == 200:
        for c in certs["certificates"]:
            mark = " [TEST KEY]" if c["isTestKey"] else ""
            print("  %s  %s%s  %s" % (c["thumbprint"][-8:], c["displayName"], mark,
                                      "usable" if c["usable"] else c.get("notUsableReason", "")))

    with open(path, "rb") as f:
        content = base64.b64encode(f.read()).decode("ascii")
    status, job = call("POST", "/v2/sign/pdf", {
        "documents": [{"name": os.path.basename(path), "content": content}],
        "level": "b-b",
    }, app_id, secret)
    if status != 202:
        raise SystemExit("sign/pdf: HTTP %s %s" % (status, job))
    print("job %s — approve it in the agent's window" % job["jobId"])
    print("batch fingerprint %s" % job["batchFingerprint"])

    while True:
        time.sleep(1)
        status, body = call("GET", job["resultUrl"], None, app_id, secret)
        if status == 202:
            print("  %s %s/%s" % (body["state"], body["completed"], body["total"]))
            continue
        if status != 200:
            raise SystemExit("failed: HTTP %s %s" % (status, body))
        out = os.path.splitext(path)[0] + "-signed.pdf"
        with open(out, "wb") as f:
            f.write(base64.b64decode(body["documents"][0]["content"]))
        print("saved %s at %s" % (out, body["documents"][0]["achievedLevel"]))
        return


if __name__ == "__main__":
    main()
