#!/usr/bin/env bash
# Diagnostic for qwen3-6-nvfp4 ↔ UUID resolution
set -e
BASE="${1:-https://litellm.carloslapao.com}"
KEY="${2:?Usage: $0 <LITELLM_BASE_URL> <API_KEY>}"
TMP=$(mktemp)
trap 'rm -f "${TMP}"' EXIT

echo "=========================================="
echo "LiteLLM slug ↕ UUID resolution diagnostic"
echo "Slug: qwen3-6-nvfp4"
echo "Base: ${BASE}"
echo ""

# Test C: GET /model/info (ALL models)
echo "--- TEST: GET /model/info (all models, keyed by model_name) ---"
curl -s -w "\nHTTP: %{http_code}\n" "${BASE}/model/info" \
  -H "Authorization: Bearer ${KEY}" > "${TMP}.full"
echo "Response: $(wc -c < "${TMP}.full") bytes"
cp "${TMP}.full" "${TMP}"

python3 << 'PYEOF'
import json, sys, os

tmp = os.environ.get("TMP_FILE", "")
if not tmp:
    print("Error: TMP_FILE not set")
    sys.exit(1)

with open(tmp) as f:
    d = json.load(f)

mi = d.get("model_info", {})
print(f"\nKeyed models count: {len(mi)}")

target = "qwen3-6-nvfp4"
found = False
for k, v in mi.items():
    kl = k.lower()
    target_lower = target.lower()
    # Match by key OR model_name
    if target_lower in kl or target_lower in str(v.get("model_name", "")).lower():
        mid = v.get("model_info", {}).get("id", "<NONE>")
        mname = v.get("model_name", "")
        print(f"✅ MATCH FOUND:")
        print(f"   key={k}")
        print(f"   model_name={mname}")
        print(f"   model_info['id']={mid}")
        print(f"   model_info type={type(mid).__name__}")
        found = True

if not found:
    print(f"\n❌ NO KEY MATCHED '{target}'")
    print("Sample keys (first 8 of {}):".format(len(mi)))
    shown = 0
    for k in list(mi.keys())[:8]:
        n = mi[k].get("model_name", "")
        mid = mi[k].get("model_info", {}).get("id", "<NONE>")
        print(f"   key={k} | name={n} | id={mid}")
        shown += 1
    
    if len(mi) > 8:
        print(f"   ... and {len(mi) - 8} more")
    
    # Check close variants
    print("\nTrying common variants:")
    for variant in ["qwen3_6_nvp4", "qwen36nvfp4", "qwen3-6", "qwen3_6"]:
        for k in list(mi.keys()):
            if variant.lower() in k.lower():
                mid = mi[k].get("model_info", {}).get("id", "<NONE>")
                print(f"   FOUND '{variant}' via substring → key={k}, id={mid}")
                break
        else:
            print(f"   ❌ '{variant}' not found either")

    # Show all unique model names
    names = []
    for v in mi.values():
        names.append(v.get("model_name", ""))
    print(f"\nAll distinct model_names ({len(set(names))}):")
    for n in sorted(set(names)):
        if n:
            print(f"   {n!r}")

PYEOF

export TMP_FILE="${TMP}"
python3 << PYEOF
import json, sys

with open("${TMP}") as f:
    d = json.load(f)

mi = d.get("model_info", {})
print(f"Keyed models count: {len(mi)}")

target = "qwen3-6-nvfp4"
found = False
for k, v in mi.items():
    kl = k.lower()
    target_lower = target.lower()
    if target_lower in kl or target_lower in str(v.get("model_name", "")).lower():
        mid = v.get("model_info", {}).get("id", "<NONE>")
        mname = v.get("model_name", "")
        print(f"✅ MATCH FOUND:")
        print(f"   key={k}")
        print(f"   model_name={mname}")
        print(f"   model_info[\"id\"]={mid}")
        print(f"   model_info type={type(mid).__name__}")
        found = True

if not found:
    print(f"Issues no key matched '{target}'")
    print("\\nSample keys (first 10 of {}):".format(len(mi)))
    shown = 0
    for k in list(mi.keys())[:10]:
        n = mi[k].get("model_name", "")
        mid = mi[k].get("model_info", {}).get("id", "<NONE>")
        print(f"   key={k} | name={n} | id={mid}")
PYEOF
