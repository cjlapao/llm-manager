#!/usr/bin/env python3
"""Diagnostic: Find qwen3-6-nvfp4 to internal UUID mapping in LiteLLM proxy."""
import sys, json, urllib.request, http.client, ssl

BASE = sys.argv[1] if len(sys.argv) > 1 else ""
KEY = sys.argv[2] if len(sys.argv) > 2 else ""
SLUG = "qwen3-6-nvfp4"

def make_request(path):
    from urllib.parse import urlparse
    parsed = urlparse(BASE.rstrip("/") + path)
    
    ctx = ssl.create_default_context()
    
    conn = http.client.HTTPSConnection(parsed.hostname, timeout=15) if parsed.scheme == "https" else None
    if conn is None:
        conn = http.client.HTTPConnection(parsed.hostname, timeout=15)
    
    headers = {
        "Authorization": f"Bearer {KEY}",
        "Content-Type": "application/json",
    }
    
    try:
        conn.request("GET", parsed.path, headers=headers)
        resp = conn.getresponse()
        body = resp.read().decode()
        code = resp.status
        conn.close()
        return code, body
    except Exception as e:
        conn.close()
        return 0, str(e)

print(f"# SLUG={SLUG}")

# A: GET /models (OpenAI compatible)
code, body = make_request("/models")
print(f"\n--- GET /models HTTP:{code} size:{len(body)} ---")
try:
    d = json.loads(body)
    items = d.get("data", [])
    print(f"Total: {len(items)} models")
    # Show entries with slug
    for m in items:
        s = json.dumps(m).lower()
        if "qwen" in s.lower():
            print(json.dumps(m, indent=2))
except Exception as e:
    print(f"Parse error: {e}")
    print(body[:500])

# B: current broken approach
code, body = make_request(f"/model/info?qwen3-6-nvfp4")
print(f"\n--- GET /model/info?qwen3-6-nvfp4 HTTP:{code} ---")
print(body[:500])

# C: ALL models via /model/info
code, body = make_request("/model/info")
print(f"\n--- GET /model/info all HTTP:{code} size:{len(body)} ---")
if code == 200:
    d = json.loads(body)
    mi = d.get("model_info", {})
    print(f"Keyed models: {len(mi)}")
    
    found = None
    for k, v in mi.items():
        kl = k.lower()
        tn = str(v.get("model_name", "")).lower()
        if SLUG.lower() in kl or SLUG.lower() in tn:
            mid = v.get("model_info", {}).get("id", "NONE")
            nm = v.get("model_name", "")
            found = (k, nm, mid)
            break
    
    if found:
        key, name, uid = found
        print(f"\n>>> FOUND: key='{key}' model_name='{name}' id='{uid}'")
    else:
        print(f"\nNO MATCH for '{SLUG}'. Sample keys:")
        for i, k in enumerate(list(mi.keys())[:8]):
            nm = mi[k].get("model_name", "")
            uid = mi[k].get("model_info", {}).get("id", "<none>")
            print(f"  #{i}: '{k}' name='{nm}' id='{uid}'")
        
        # show model_name values specifically  
        names = set()
        for v in mi.values():
            n = v.get("model_name", "")
            if n: names.add(n)
        print(f"\nDistinct model_names ({len(names)}):")
        for n in sorted(names)[:20]:
            print(f"  {n!r}")
