#!/usr/bin/env bash
# modelcli — manage all AI model containers on the PGX server
# Usage:
#   modelcli list                   — show all models + ports
#   modelcli status                 — show running containers
#   modelcli start  <model>         — start a model (others keep running)
#   modelcli stop   <model|all>     — stop one or all models
#   modelcli swap   <model>         — stop all, then start one (GPU-safe)
#   modelcli update <model|all>     — pull latest weights from HuggingFace
#   modelcli comfyui <start|stop>
#   modelcli hotspot [status|stop|restart]
set -euo pipefail

INSTALL_DIR="/opt/ai-server"
LLM_DIR="${INSTALL_DIR}/llm-compose"
SETTINGS_FILE="${INSTALL_DIR}/models.json"
ACTIVE_FLUX_FILE="${INSTALL_DIR}/comfyui/.active-model"

# Load LLM model registry + HF repos from models.json at startup
declare -A MODEL_YML=() MODEL_CONTAINER=() MODEL_PORT=() MODEL_HF_REPO=() MODEL_NAME=()
HF_CACHE_DIR="${INSTALL_DIR}/models"
if [[ -f "${SETTINGS_FILE}" ]]; then
    _reg=$(python3 - "${SETTINGS_FILE}" <<'PYEOF'
import json, sys
data = json.load(open(sys.argv[1]))
hf_cache = data.get("hf_cache_dir", "/opt/ai-server/models")
print(f"HF_CACHE_DIR='{hf_cache}'")
for k, m in data["models"].items():
    repo = m.get("hf_repo", "").replace("'", "'\\''")
    name = m.get("name", k).replace("'", "'\\''")
    print(f"MODEL_HF_REPO['{k}']='{repo}'")
    print(f"MODEL_NAME['{k}']='{name}'")
    if m.get("type") == "llm":
        yml  = m.get("yml",       "").replace("'", "'\\''")
        ctr  = m.get("container", "").replace("'", "'\\''")
        port = str(m.get("port", 0))
        print(f"MODEL_YML['{k}']='{yml}'")
        print(f"MODEL_CONTAINER['{k}']='{ctr}'")
        print(f"MODEL_PORT['{k}']='{port}'")
PYEOF
)
    eval "$_reg"
else
    echo "Warning: ${SETTINGS_FILE} not found. Run init.sh to regenerate." >&2
fi

# Flux image-gen models — run via ComfyUI, not vLLM
FLUX_MODELS=("flux-schnell" "flux-dev")
declare -A FLUX_CHECKPOINT=(
    [flux-schnell]="flux1-schnell.safetensors"
    [flux-dev]="flux1-dev.safetensors"
)

# 3D generation models — weights in comfyui/<dir>/
ACTIVE_3D_FILE="/opt/ai-server/comfyui/.active-3d"
THREED_MODELS=("hunyuan3d" "trellis")
declare -A THREED_DIR=(
    [hunyuan3d]="hunyuan3d"
    [trellis]="trellis"
)


# ComfyUI / Embed / Rerank / Speech containers
COMFYUI_CONTAINER="comfyui-flux"
EMBED_CONTAINER="llm-embed"
RERANK_CONTAINER="llm-rerank"
WHISPER_CONTAINER="whisper-stt"
KOKORO_CONTAINER="kokoro-tts"

# Hotspot tracking
ACTIVE_HOTSPOT_FILE="${LLM_DIR}/.active-hotspot"

COMMAND="${1:-list}"
TARGET="${2:-}"

# --- helpers ---

_is_flux() { [[ "${TARGET}" == flux-* ]]; }

_is_hf_cached() {
    # Returns "yes" if the HuggingFace cache dir contains the given repo.
    # HF cache layout: <cache>/models--<org>--<name>/snapshots/
    local repo="$1"
    [[ -z "$repo" ]] && echo "n/a" && return
    local dir_name="models--$(echo "$repo" | tr '/' '--')"
    if [[ -d "${HF_CACHE_DIR}/${dir_name}/snapshots" ]]; then
        echo "yes"
    else
        echo "no"
    fi
}

_container_status() {
    # Returns the container status without the docker inspect [] stdout pollution.
    # When a container doesn't exist, docker inspect exits non-zero AND writes []
    # to stdout; using || echo "stopped" would concatenate both outputs.
    # We capture exit code separately to avoid that.
    local name=$1
    local s
    s=$(docker inspect --format '{{.State.Status}}' "${name}" 2>/dev/null)
    [[ $? -eq 0 ]] && echo "${s}" || echo "stopped"
}

_llm_status() {
    local model=$1
    _container_status "${MODEL_CONTAINER[$model]}"
}

_flux_status() {
    local model=$1
    local active=""
    [[ -f "${ACTIVE_FLUX_FILE}" ]] && active=$(cat "${ACTIVE_FLUX_FILE}")
    local comfyui_state
    comfyui_state=$(_container_status "comfyui-flux")
    if [[ "${active}" == "${model}" && "${comfyui_state}" == "running" ]]; then
        echo "active"
    elif [[ "${comfyui_state}" == "running" ]]; then
        echo "standby"
    else
        echo "stopped"
    fi
}

_stop_llm() {
    local model=$1
    echo "  stopping ${model}..."
    docker compose -f "${LLM_DIR}/${MODEL_YML[$model]}" down 2>/dev/null || true
}

_start_llm() {
    local model=$1
    echo "  starting ${model} on port ${MODEL_PORT[$model]}..."
    docker compose -f "${LLM_DIR}/${MODEL_YML[$model]}" up -d
}

_stop_all_llms() {
    for model in "${!MODEL_YML[@]}"; do _stop_llm "$model"; done
}

_activate_flux() {
    local model=$1
    local checkpoint="${FLUX_CHECKPOINT[$model]}"
    echo "  stopping all LLM containers to free GPU memory..."
    _stop_all_llms
    echo "  activating ${model} (${checkpoint}) in ComfyUI..."
    echo "${model}" > "${ACTIVE_FLUX_FILE}"
    echo "Done. Load '${checkpoint}' from the unet/ directory in your ComfyUI workflow."
    echo "ComfyUI UI: http://localhost:8188"
}

_deactivate_flux() {
    rm -f "${ACTIVE_FLUX_FILE}"
    echo "  Flux model deactivated. ComfyUI is still running."
}

_is_3d() {
    for m in "${THREED_MODELS[@]}"; do [[ "$TARGET" == "$m" ]] && return 0; done
    return 1
}

_threed_status() {
    local model=$1
    local active=""
    [[ -f "${ACTIVE_3D_FILE}" ]] && active=$(cat "${ACTIVE_3D_FILE}")
    if [[ "${active}" == "${model}" ]]; then
        echo "active"
    else
        echo "stopped"
    fi
}

_activate_3d() {
    local model=$1
    echo "  stopping all LLM containers to free GPU memory..."
    _stop_all_llms
    rm -f "${ACTIVE_FLUX_FILE}"
    echo "  activating ${model} (weights in comfyui/${THREED_DIR[$model]}/)..."
    echo "${model}" > "${ACTIVE_3D_FILE}"
    echo "Done. Load the ${model} weights from comfyui/${THREED_DIR[$model]}/ in your pipeline."
}

_deactivate_3d() {
    rm -f "${ACTIVE_3D_FILE}"
    echo "  3D model deactivated."
}

_docker_stop() {
    local name=$1 label=$2
    local state
    state=$(_container_status "${name}")
    if [[ "${state}" == "running" ]]; then
        echo "  stopping ${label}..."
        docker stop "${name}"
    else
        echo "  ${label} is not running (${state})."
    fi
}

_docker_start() {
    local name=$1 label=$2 port=$3
    echo "  starting ${label}..."
    docker start "${name}"
    echo "  ${label} is up on port ${port}."
}

_compose_start() {
    # Start a profile-based service via docker compose so the network is
    # (re)created correctly — "docker start" fails if the network was destroyed.
    local profile=$1 service=$2 label=$3 port=$4
    local compose_file="${INSTALL_DIR}/docker-compose.yml"
    echo "  starting ${label}..."
    docker compose -f "${compose_file}" --profile "${profile}" up -d "${service}"
    echo "  ${label} is up on port ${port}."
}

_stop_comfyui()  { _docker_stop "${COMFYUI_CONTAINER}" "ComfyUI"; }
_start_comfyui() { _compose_start comfyui comfyui "ComfyUI" "8188"; }

_stop_embed()   { _docker_stop  "${EMBED_CONTAINER}"  "embed"; }
_start_embed()  { _docker_start "${EMBED_CONTAINER}"  "embed"  "${PORT_EMBED:-8020}"; }

_stop_rerank()  { _docker_stop  "${RERANK_CONTAINER}" "rerank"; }
_start_rerank() { _docker_start "${RERANK_CONTAINER}" "rerank" "${PORT_RERANK:-8021}"; }

_stop_whisper()  { _docker_stop "${WHISPER_CONTAINER}" "whisper-stt"; }
_start_whisper() { _compose_start speech  whisper-stt  "whisper-stt" "${PORT_WHISPER:-8004}"; }

_stop_kokoro()   { _docker_stop "${KOKORO_CONTAINER}" "kokoro-tts"; }
_start_kokoro()  { _compose_start speech  kokoro-tts   "kokoro-tts"  "${PORT_KOKORO:-8005}"; }

# ----------------------------------------------------------

case "$COMMAND" in
  list)
    printf "%-20s %-34s %-6s %-10s %-6s\n" "SLUG" "NAME" "PORT" "STATUS" "CACHED"
    printf "%-20s %-34s %-6s %-10s %-6s\n" "----" "----" "----" "------" "------"
    for model in $(echo "${!MODEL_YML[@]}" | tr ' ' '\n' | sort); do
        printf "%-20s %-34s %-6s %-10s %-6s\n" \
            "$model" \
            "${MODEL_NAME[$model]:-$model}" \
            "${MODEL_PORT[$model]}" \
            "$(_llm_status "$model")" \
            "$(_is_hf_cached "${MODEL_HF_REPO[$model]}")"
    done
    ;;

  status)
    echo "Running AI containers:"
    docker ps --filter "name=llm-" --filter "name=comfyui-flux" \
        --format "  {{.Names}}\t{{.Status}}\t{{.Ports}}"
    if [[ -f "${ACTIVE_FLUX_FILE}" ]]; then
        echo "  Active Flux model: $(cat "${ACTIVE_FLUX_FILE}")"
    fi
    if [[ -f "${ACTIVE_3D_FILE}" ]]; then
        echo "  Active 3D model: $(cat "${ACTIVE_3D_FILE}")"
    fi
    if [[ -f "${ACTIVE_HOTSPOT_FILE}" ]]; then
        echo "  Active hotspot model: $(cat "${ACTIVE_HOTSPOT_FILE}")"
    fi
    ;;

  start)
    [[ -z "$TARGET" ]] && { echo "Usage: modelcli start <model>"; exit 1; }
    if _is_flux; then
        [[ -z "${FLUX_CHECKPOINT[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        _activate_flux "$TARGET"
    elif _is_3d; then
        [[ -z "${THREED_DIR[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        _activate_3d "$TARGET"
    else
        [[ -z "${MODEL_YML[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        _start_llm "$TARGET"
        echo "Started. ${TARGET} is available on port ${MODEL_PORT[$TARGET]}."
    fi
    ;;

  stop)
    [[ -z "$TARGET" ]] && { echo "Usage: modelcli stop <model|all>"; exit 1; }
    if [[ "$TARGET" == "all" ]]; then
        _stop_all_llms
        _deactivate_flux
        _deactivate_3d
        rm -f "${ACTIVE_HOTSPOT_FILE}"
        echo "All models stopped."
    elif _is_flux; then
        [[ -z "${FLUX_CHECKPOINT[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        _deactivate_flux
        echo "Flux model deactivated."
    elif _is_3d; then
        [[ -z "${THREED_DIR[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        _deactivate_3d
        echo "3D model deactivated."
    else
        [[ -z "${MODEL_YML[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        _stop_llm "$TARGET"
        active=$(cat "${ACTIVE_HOTSPOT_FILE}" 2>/dev/null || echo "")
        if [[ "${active}" == "${TARGET}" ]]; then
            rm -f "${ACTIVE_HOTSPOT_FILE}"
        fi
        echo "Stopped ${TARGET}."
    fi
    ;;

  swap)
    [[ -z "$TARGET" ]] && { echo "Usage: modelcli swap <model>"; exit 1; }
    if _is_flux; then
        [[ -z "${FLUX_CHECKPOINT[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        echo "Swapping to ${TARGET}..."
        _deactivate_3d
        _activate_flux "$TARGET"
        echo "Swap complete. Use '${FLUX_CHECKPOINT[$TARGET]}' in your ComfyUI workflow."
    elif _is_3d; then
        [[ -z "${THREED_DIR[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        echo "Swapping to ${TARGET}..."
        _deactivate_flux
        _activate_3d "$TARGET"
        echo "Swap complete. ${TARGET} weights are in comfyui/${THREED_DIR[$TARGET]}/."
    else
        [[ -z "${MODEL_YML[$TARGET]+x}" ]] && { echo "Unknown model: $TARGET"; exit 1; }
        echo "Stopping all models..."
        _stop_all_llms
        _deactivate_flux
        _deactivate_3d
        # Drop OS page cache so model files from the previous run (or a failed
        # load attempt) don't eat into the unified memory budget before the new
        # model's CUDA tensors are allocated.  Critical on GB10: the 71 GB param
        # pre-allocation leaves only ~34 GB for the CPU shard staging buffer, and
        # stale page cache from a prior attempt pushes it over the edge.
        echo "  dropping OS page cache..."
        sync && echo 3 | tee /proc/sys/vm/drop_caches > /dev/null 2>&1 || true
        echo "Starting ${TARGET}..."
        _start_llm "$TARGET"
        echo "$TARGET" > "${ACTIVE_HOTSPOT_FILE}"
        echo "Swap complete. ${TARGET} is on port ${MODEL_PORT[$TARGET]}."
    fi
    ;;

  update)
    [[ -z "$TARGET" ]] && { echo "Usage: modelcli update <model|all>"; exit 1; }
    [[ -z "${HF_TOKEN:-}" ]] && { echo "Error: HF_TOKEN is not set. Export it first: export HF_TOKEN=hf_..."; exit 1; }

    _update_model() {
        local key=$1
        local repo="${MODEL_HF_REPO[$key]:-}"
        if [[ -z "$repo" ]]; then
            echo "  ✗ Unknown model or no HF repo configured: $key"
            return 1
        fi
        echo "  Updating $key ($repo)..."
        if HF_HOME="${HF_CACHE_DIR}" hf download "$repo" \
                --token "${HF_TOKEN}" \
                --quiet 2>&1; then
            echo "  ✓ $key updated"
        else
            echo "  ✗ Failed to update $key"
            return 1
        fi
    }

    if [[ "$TARGET" == "all" ]]; then
        echo "Updating all models from HuggingFace..."
        failed=0
        for key in "${!MODEL_HF_REPO[@]}"; do
            _update_model "$key" || (( failed++ )) || true
        done
        echo ""
        (( failed > 0 )) \
            && echo "Update finished with ${failed} failure(s)." \
            || echo "All models updated successfully."
    else
        _update_model "$TARGET"
    fi
    ;;

  hotspot)
    HOTSPOT_CMD="${TARGET:-status}"
    case "$HOTSPOT_CMD" in
      status|"")
        active=$(cat "${ACTIVE_HOTSPOT_FILE}" 2>/dev/null || echo "")
        if [[ -z "$active" ]]; then
            echo "No active hotspot model."
        else
            [[ -z "${MODEL_YML[$active]+x}" ]] && { echo "Hotspot file contains unknown model '${active}'; clearing."; rm -f "${ACTIVE_HOTSPOT_FILE}"; exit 1; }
            state=$(docker inspect --format '{{.State.Status}}' "${MODEL_CONTAINER[$active]}" 2>/dev/null || echo "stopped")
            echo "Active hotspot: ${active}  (port ${MODEL_PORT[$active]})  status=${state}"
        fi
        ;;
      stop)
        active=$(cat "${ACTIVE_HOTSPOT_FILE}" 2>/dev/null || echo "")
        if [[ -z "$active" ]]; then
            echo "No active hotspot model."
            exit 0
        fi
        [[ -z "${MODEL_YML[$active]+x}" ]] && { echo "Hotspot file contains unknown model '${active}'; clearing."; rm -f "${ACTIVE_HOTSPOT_FILE}"; exit 1; }
        _stop_llm "$active"
        rm -f "${ACTIVE_HOTSPOT_FILE}"
        echo "Done."
        ;;
      restart)
        active=$(cat "${ACTIVE_HOTSPOT_FILE}" 2>/dev/null || echo "")
        if [[ -z "$active" ]]; then
            echo "No active hotspot model."
            exit 0
        fi
        [[ -z "${MODEL_YML[$active]+x}" ]] && { echo "Hotspot file contains unknown model '${active}'; clearing."; rm -f "${ACTIVE_HOTSPOT_FILE}"; exit 1; }
        echo "Restarting ${active}..."
        _stop_llm "$active"
        _start_llm "$active" || { rm -f "${ACTIVE_HOTSPOT_FILE}"; echo "Failed to start ${active}; hotspot cleared."; exit 1; }
        echo "Done. ${active} is running on port ${MODEL_PORT[$active]}."
        ;;
      *)
        echo "Usage: modelcli hotspot [status|stop|restart]"
        exit 1
        ;;
    esac
    ;;

  logs)
    [[ -z "${TARGET}" ]] && { echo "Usage: modelcli logs <model|service>"; exit 1; }
    # Resolve model/service name to container name
    _logs_container=""
    case "${TARGET}" in
      comfyui|flux)       _logs_container="${COMFYUI_CONTAINER}" ;;
      embed)              _logs_container="${EMBED_CONTAINER}" ;;
      rerank)             _logs_container="${RERANK_CONTAINER}" ;;
      whisper)            _logs_container="${WHISPER_CONTAINER}" ;;
      kokoro)             _logs_container="${KOKORO_CONTAINER}" ;;
      litellm)            _logs_container="litellm" ;;
      swap-api|swapapi)   _logs_container="swap-api" ;;
      open-webui|webui)   _logs_container="open-webui" ;;
      mcp)                _logs_container="mcpo" ;;
      *)
        if [[ -n "${MODEL_CONTAINER[${TARGET}]+x}" ]]; then
          _logs_container="${MODEL_CONTAINER[$TARGET]}"
        else
          echo "Unknown model or service: ${TARGET}"
          echo "Known services: comfyui, embed, rerank, whisper, kokoro, litellm, swap-api, open-webui, mcp"
          exit 1
        fi
        ;;
    esac
    echo "  streaming logs for ${_logs_container} (Ctrl+C to stop)..."
    exec docker logs -f "${_logs_container}"
    ;;

  comfyui)
    case "${TARGET:-}" in
      start) _start_comfyui ;;
      stop)  _stop_comfyui  ;;
      *)     echo "Usage: modelcli comfyui <start|stop>"; exit 1 ;;
    esac
    ;;

  embed)
    case "${TARGET:-}" in
      start) _start_embed ;;
      stop)  _stop_embed  ;;
      *)     echo "Usage: modelcli embed <start|stop>"; exit 1 ;;
    esac
    ;;

  rerank)
    case "${TARGET:-}" in
      start) _start_rerank ;;
      stop)  _stop_rerank  ;;
      *)     echo "Usage: modelcli rerank <start|stop>"; exit 1 ;;
    esac
    ;;

  rag)
    case "${TARGET:-}" in
      start)
        _start_embed
        _start_rerank
        ;;
      stop)
        _stop_embed
        _stop_rerank
        ;;
      *)     echo "Usage: modelcli rag <start|stop>"; exit 1 ;;
    esac
    ;;

  speech)
    case "${TARGET:-}" in
      start)
        _start_whisper
        _start_kokoro
        ;;
      stop)
        _stop_whisper
        _stop_kokoro
        ;;
      *)     echo "Usage: modelcli speech <start|stop>"; exit 1 ;;
    esac
    ;;

  mem)
    python3 - "${SETTINGS_FILE}" "${LLM_DIR}" "${HF_CACHE_DIR}" "${TARGET:-}" <<'MEMEOF'
import json, os, sys, re, urllib.request, urllib.error

settings_file, llm_dir, hf_cache_dir, target = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]

try:
    data = json.load(open(settings_file))
except Exception as e:
    print(f"Cannot read {settings_file}: {e}", file=sys.stderr); sys.exit(1)

models   = data["models"]
hf_token = os.environ.get("HF_TOKEN") or os.environ.get("HUGGING_FACE_HUB_TOKEN")

# bytes-per-parameter for each quantisation scheme
QUANT_BYTES = {
    "nvfp4": 0.55,          # 4-bit weights + FP8 block-scaling overhead
    "fp4":   0.5,
    "fp8":   1.0,
    "int8":  1.0,
    "bf16":  2.0, "bfloat16": 2.0,
    "fp16":  2.0, "float16":  2.0,
    "int4":  0.5, "awq": 0.5, "gptq": 0.5,
    "modelopt_mixed": 0.6,  # mixed FP8/INT4 — approximate
}
KV_BYTES = {"fp8": 1.0, "fp16": 2.0, "bf16": 2.0, "bfloat16": 2.0, "auto": 2.0}
SHOW_CTX = [4_096, 32_768, 131_072, 262_144]

def detect_quant(hf_repo, yml):
    n = (hf_repo or "").lower()
    for k in ("nvfp4", "fp8", "fp4", "int4", "gptq", "awq", "int8"):
        if k in n: return k
    m = re.search(r'--quantization[=\s]+(\S+)', yml or "")
    if m: return m.group(1).lower()
    m = re.search(r'--dtype[=\s]+(\S+)', yml or "")
    if m: return m.group(1).lower()
    return "bf16"

def local_config(hf_repo):
    base = os.path.join(hf_cache_dir, "models--" + hf_repo.replace("/", "--"), "snapshots")
    if not os.path.isdir(base): return None
    for snap in sorted(os.listdir(base)):
        p = os.path.join(base, snap, "config.json")
        if os.path.exists(p):
            try: return json.load(open(p))
            except Exception: pass
    return None

def fetch_config(hf_repo):
    url = f"https://huggingface.co/{hf_repo}/resolve/main/config.json"
    req = urllib.request.Request(url, headers={"User-Agent": "modelcli/1.0"})
    if hf_token: req.add_header("Authorization", f"Bearer {hf_token}")
    try:
        with urllib.request.urlopen(req, timeout=12) as r:
            return json.loads(r.read()), "hf-api"
    except urllib.error.HTTPError as e:
        return None, f"HTTP {e.code}" + (" (set HF_TOKEN)" if e.code == 401 else "")
    except Exception as e:
        return None, str(e)[:50]

def yml_int(t, flag):
    m = re.search(rf'--{flag}[=\s]+(\d+)', t or "")
    return int(m.group(1)) if m else None

def yml_str(t, flag):
    m = re.search(rf'--{flag}[=\s]+(\S+)', t or "")
    return m.group(1) if m else None

def est_params(cfg):
    H  = cfg.get("hidden_size", 0)
    L  = cfg.get("num_hidden_layers", 0)
    V  = cfg.get("vocab_size", 0)
    nh = cfg.get("num_attention_heads", 1)
    kh = cfg.get("num_key_value_heads", nh)
    ff = cfg.get("intermediate_size", 0)
    hd = cfg.get("head_dim", H // max(nh, 1))
    ne = cfg.get("num_experts", cfg.get("num_local_experts", 1))
    attn  = L * H * (nh * hd + 2 * kh * hd + nh * hd)   # Q K V O projections
    ffn   = L * ff * H * 3 * max(ne, 1)                   # gate + up + down, all experts
    embed = V * H * 2                                       # embedding + lm_head
    norms = L * H * 4
    return attn + ffn + embed + norms

def kv_bpt(cfg, kv_b):
    L  = cfg.get("num_hidden_layers", 0)
    nh = cfg.get("num_attention_heads", 1)
    kh = cfg.get("num_key_value_heads", nh)
    H  = cfg.get("hidden_size", 0)
    hd = cfg.get("head_dim", H // max(nh, 1))
    return 2 * L * kh * hd * kv_b   # K + V, all layers

def fmt(n):
    gb = n / 1e9
    if gb < 1:   return f"{gb*1024:.0f}MB"
    if gb < 100: return f"{gb:.1f}GB"
    return f"{gb:.0f}GB"

# ── Print table ───────────────────────────────────────────────────────────────
CW, NW, QW, WW, XW = 20, 32, 7, 8, 9
ctx_hdr = "  ".join(f"{'KV@'+str(c//1024)+'K':>{XW}}" for c in SHOW_CTX)
hdr = f"  {'SLUG':<{CW}} {'NAME':<{NW}} {'QUANT':<{QW}} {'WEIGHTS':>{WW}}  {ctx_hdr}"
print(); print(hdr); print("  " + "-" * (len(hdr) - 2))

llm_models = {k: v for k, v in models.items()
              if v.get("type") == "llm" and (not target or k == target)}
if not llm_models:
    print(f"  No LLM model found{': ' + target if target else ''}."); sys.exit(0)

for slug, m in sorted(llm_models.items()):
    hf_repo  = m.get("hf_repo", "")
    name     = m.get("name", slug)
    yml_text = ""
    try: yml_text = open(os.path.join(llm_dir, m.get("yml", ""))).read()
    except Exception: pass

    quant   = detect_quant(hf_repo, yml_text)
    q_b     = QUANT_BYTES.get(quant, 2.0)
    kv_dt   = yml_str(yml_text, "kv-cache-dtype") or "bf16"
    kv_b    = KV_BYTES.get(kv_dt.lower(), 2.0)
    max_len = yml_int(yml_text, "max-model-len")

    cfg, src = local_config(hf_repo), "cache"
    if cfg is None:
        cfg, src = fetch_config(hf_repo)

    if cfg is None:
        print(f"  {slug:<{CW}} {name:<{NW}} {quant:<{QW}} {'—':>{WW}}  [{src}]")
        continue

    if max_len is None:
        max_len = cfg.get("max_position_embeddings") or cfg.get("model_max_length") or 0

    params  = est_params(cfg)
    w_bytes = params * q_b
    bpt     = kv_bpt(cfg, kv_b)

    cols = []
    for ctx in SHOW_CTX:
        if max_len and ctx > max_len:
            cols.append(f"{'—':>{XW}}")
        else:
            cols.append(f"{fmt(bpt * ctx):>{XW}}")

    tag = f" [fetched]" if src == "hf-api" else ""
    print(f"  {slug:<{CW}} {name:<{NW}} {quant:<{QW}} {fmt(w_bytes):>{WW}}  {'  '.join(cols)}{tag}")

print()
print(f"  Weights  = estimated param count × quant bytes/param.")
print(f"  KV@Nk    = additional VRAM for one request at that context length (kv-dtype: {kv_dt}).")
print(f"  Total    ≈ Weights + KV@context + ~5–10% vLLM overhead.")
MEMEOF
    ;;

  *)
    echo "Usage: $0 <command> [model|subcommand]"
    echo ""
    echo "Commands:"
    echo "  list                     — show all models + ports"
    echo "  mem    [model]           — estimate GPU memory: weights + KV cache per context"
    echo "  status                   — show running containers"
    echo "  start  <model>           — start a model (others keep running)"
    echo "  stop   <model|all>       — stop one or all models"
    echo "  swap   <model>           — GPU-safe: stop all, then start one"
    echo "  update <model|all>       — pull latest weights from HuggingFace"
    echo "  logs   <model|service>   — stream container logs (Ctrl+C to stop)"
    echo "  hotspot [status|stop|restart]"
    echo "  comfyui <start|stop>"
    echo "  embed   <start|stop>"
    echo "  rerank  <start|stop>"
    echo "  rag     <start|stop>     — start/stop embed + rerank together"
    echo "  speech  <start|stop>     — start/stop whisper-stt + kokoro-tts"
    exit 1
    ;;
esac
