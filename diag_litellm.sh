#!/bin/bash
# Quick LiteLLM diagnostic - shows how each endpoint resolves a slug to UUID
set -e
BASE="$1"
KEY="$2"
if [ -z "$BASE" ] || [ -z "$KEY" ]; then
  echo "Usage: $0 <LITELLM_BASE_URL> <API_KEY>"
  exit 1
fi
SLUG="qwen3-6-nvfp4"
echo "=========================================="
echo "Diagnostic for slug: ${SLUG}"
echo "Base: ${BASE}"
echo "=========================================="
echo ""

echo "--- Test A: GET /model/info?qwen3-6-nvfp4 (current broken approach) ---"
curl -s -w "\n\nHTTP_CODE=%{http_code}\n" \
  "${BASE}/model/info?qwen3-6-nvfp4" \
  -H "Authorization: Bearer ${KEY}" | head -c 1000
echo ""
echo ""

echo "--- Test B: GET /model/info (ALL models - may return huge payload) ---"
echo "(showing first model's model_info.id if found)"
BODY=$(curl -s "${BASE}/model/info" \
  -H "Authorization: Bearer ${KEY}")
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/model/info" \
  -H "Authorization: Bearer ${KEY}")
echo "HTTP_CODE=${HTTP_CODE}"
echo "Response length: ${#BODY} bytes"
if [ "$HTTP_CODE" = "200" ]; then
  # Try to extract id for our slug using jq-like grep
  echo "Looking for '${SLUG}' in keys..."
  echo "$BODY" | python3 -c "
import sys, json
data = json.load(sys.stdin)
mi = data.get('model_info', {})
found_key = None
for k in mi.keys():
    if '${SLUG}' in k.lower() or 'qwen' in k.lower():
        print(f'  MATCH: key=\"{k}\"')
        mid = mi[k].get('model_info', {}).get('id', '<no id>')
        print(f'            model_info[\"id\"]={mid}')
        print(f'  Full key values (first 500 chars): {str(mi[k].values())[:500]}')
        break
else:
    print('No key matched')
    print(f'All keys: {list(mi.keys())[:10]}...')
" 2>/dev/null || echo "(jq-like parsing not available, show raw preview)" >&2
fi
echo ""

echo "--- Test C: GET /models (OpenAI format) ---"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/models" \
  -H "Authorization: Bearer ${KEY}")
echo "HTTP_CODE=${HTTP_CODE}"
curl -s "${BASE}/models" \
  -H "Authorization: Bearer ${KEY}" | python3 -c "
import sys, json
# Show first few entries to understand ID format
data = json.loads(sys.stdin.read())
print(f'Total models: {len(data.get(\"data\", []))}')
# Find qwen3-6
for m in data.get('data', []):
    if 'qwen3-6' in str(m) or 'qwen3_6' in str(m):
        print(f'Match: {json.dumps(m, indent=2)}')
        break
" 2>/dev/null | head -c 1000
echo ""
