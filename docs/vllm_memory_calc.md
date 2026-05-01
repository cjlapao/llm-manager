# vLLM Memory Calculator — Implementation Specification

Target hardware: NVIDIA DGX Spark GB10 (128 GB unified LPDDR5x, ~119 GB reported by OS).

This document describes how to calculate exactly how much GPU memory a vLLM model instance will consume, and how to derive the correct `gpu_memory_utilization` parameter for safe multi-model coexistence on a single machine.

## Core Principle

On the DGX Spark, GPU memory and CPU memory are the **same physical pool**. vLLM's `gpu_memory_utilization` is a fraction of **total** GPU memory (always ~119 GB), not free memory. vLLM pre-allocates this amount at startup as one contiguous block. It does NOT grow or shrink at runtime.

When running multiple vLLM instances, each calculates its allocation against the same 119 GB total independently. The sum of all allocations must leave enough room for the OS, other processes, and a safety margin.

---

## System Constants

```
TOTAL_GPU_MB        = 121,856   (119 GB as reported by nvidia-smi / torch.cuda)
OS_RESERVE_MB       =   7,168   (7 GB — kernel, system services, Docker daemon)
FILE_BUFFER_MB      =   5,120   (5 GB — Linux page cache, disk buffers)
SYSTEM_RESERVE_MB   =  12,288   (OS_RESERVE + FILE_BUFFER = 12 GB)
USABLE_MB           = 109,568   (TOTAL_GPU_MB - SYSTEM_RESERVE_MB)
EARLYOOM_THRESHOLD  =       3   (percent — earlyoom kills at 3% free)
EARLYOOM_RESERVE_MB =   3,656   (3% of TOTAL_GPU_MB)
SAFE_USABLE_MB      = 105,912   (USABLE_MB - EARLYOOM_RESERVE_MB)
```

---

## Memory Components

Total memory for a model = sum of all components below.

### 1. Model Weights

The dominant cost. Depends on total parameter count and quantization format.

```
weights_mb = total_params_billions × bytes_per_param × 1024

bytes_per_param:
  BF16  = 2.0
  FP8   = 1.0
  NVFP4 = 0.5
  GGUF varies by quant level (not applicable to vLLM)
```

**Important for MoE models**: `total_params` is the FULL parameter count, NOT active parameters. All experts must reside in memory even though only a subset activates per token.

Examples:
- Qwen3.6-35B-A3B FP8:  35 × 1.0 × 1024 = 35,840 MB (~35 GB)
- Qwen3.6-35B-A3B NVFP4: 35 × 0.5 × 1024 = 17,920 MB (~18 GB)
- Qwen3.6-27B FP8:      27 × 1.0 × 1024 = 27,648 MB (~27 GB)
- Gemma 4 26B-A4B FP8:  26 × 1.0 × 1024 = 26,624 MB (~26 GB)

### 2. KV Cache

Scales linearly with context length, number of concurrent sequences, and architecture. This is where model architecture matters most.

```
kv_per_token_bytes = 2 × num_kv_heads × head_dim × num_attention_layers × kv_dtype_bytes

kv_dtype_bytes:
  BF16       = 2
  FP8 (e4m3) = 1    (use --kv-cache-dtype fp8_e4m3)
  FP8 (e5m2) = 1

kv_cache_mb = (kv_per_token_bytes × max_context_length × max_concurrent_sequences) / (1024 × 1024)
```

**Critical**: only count ATTENTION layers, not all layers. Hybrid models (Qwen3.5/3.6 with GatedDeltaNet) have far fewer attention layers than total layers. GDN/SSM layers use a fixed-size recurrent state that does NOT scale with context length.

Architecture reference for models in our fleet:

```
Qwen3.6-35B-A3B:
  total_layers:     40
  attention_layers:  10   (only these contribute to KV cache)
  gdn_layers:       30   (fixed recurrent state, ~50 MB total, not per-token)
  num_kv_heads:       2
  head_dim:         256
  kv_per_token (FP8): 2 × 2 × 256 × 10 × 1 = 10,240 bytes (10 KB)

Qwen3.6-27B:
  total_layers:     48
  attention_layers:  16
  gdn_layers:       32
  num_kv_heads:       4
  head_dim:         128
  kv_per_token (FP8): 2 × 4 × 128 × 16 × 1 = 16,384 bytes (16 KB)

Qwen3.5-35B-A3B:
  (same architecture as Qwen3.6-35B-A3B)
  kv_per_token (FP8): 10,240 bytes (10 KB)

Qwen3-Coder-Next (80B):
  total_layers:     48
  attention_layers:  10   (similar hybrid architecture)
  gdn_layers:       38
  num_kv_heads:       2
  head_dim:         256
  kv_per_token (FP8): 2 × 2 × 256 × 10 × 1 = 10,240 bytes (10 KB)

Gemma 4 31B:
  total_layers:     30
  attention_layers:  30   (ALL layers are attention — no hybrid)
  num_kv_heads:       8
  head_dim:         256
  kv_per_token (FP8): 2 × 8 × 256 × 30 × 1 = 122,880 bytes (120 KB)

Gemma 4 26B-A4B:
  total_layers:     26
  attention_layers:  26   (ALL layers are attention)
  num_kv_heads:       8
  head_dim:         256
  kv_per_token (FP8): 2 × 8 × 256 × 26 × 1 = 106,496 bytes (104 KB)

Qwen3-Embedding-0.6B:
  (no KV cache needed — encoder model)
  kv_per_token: 0

Qwen3-Reranker-0.6B:
  (no KV cache needed — encoder model)
  kv_per_token: 0
```

Notice the massive difference: Qwen3.6-35B-A3B uses 10 KB per token, Gemma 4 26B uses 104 KB per token. At 262K context, that's 2.6 GB vs 27 GB. This is why Gemma models need reduced context when running alongside other models.

### 3. GDN Recurrent State (Hybrid Models Only)

Qwen3.5/3.6 hybrid models have GatedDeltaNet layers that maintain a fixed-size recurrent state per sequence. This does NOT scale with context length.

```
gdn_state_per_sequence_mb ≈ 50   (fixed, regardless of context length)
gdn_total_mb = gdn_state_per_sequence_mb × max_concurrent_sequences
```

This is small enough to fold into the off-budget estimate, but the implementation should account for it explicitly.

### 4. Prefix Cache Overhead

When `--enable-prefix-caching` is on (recommended), vLLM caches KV blocks for repeated prompt prefixes. Over long agentic sessions, this grows.

```
prefix_cache_mb = 1024   (1 GB flat estimate for typical agentic workloads)

For heavy multi-turn agentic sessions with large system prompts:
prefix_cache_mb = 2048   (2 GB)

For simple single-turn or embedding models:
prefix_cache_mb = 0
```

### 5. MTP Speculative Decoding Overhead

Multi-Token Prediction adds memory for draft heads, draft activation buffers, and verification buffers. The cost scales with **active parameters**, not total parameters.

```
For MoE models:
  active_params = active_experts × params_per_expert + shared_expert_params
  
For dense models:
  active_params = total_params

mtp_head_mb = active_params_billions × 500      (0.5 GB per billion active)
mtp_draft_mb = active_params_billions × 333 × num_speculative_tokens
mtp_verify_mb = active_params_billions × 167 × (num_speculative_tokens + 1)

mtp_total_mb = mtp_head_mb + mtp_draft_mb + mtp_verify_mb
```

Simplified reference table:

```
Model                    | Active | MTP Tokens | MTP Overhead
Qwen3.6-35B-A3B (MoE)   |   3B   |     2      |  ~2,000 MB
Qwen3.6-35B-A3B (MoE)   |   3B   |     3      |  ~2,700 MB
Qwen3.6-27B (dense)      |  27B   |     2      |  ~5,500 MB
Qwen3.6-27B (dense)      |  27B   |     3      |  ~7,500 MB
Qwen3-Coder-Next (MoE)   |   3B   |     2      |  ~2,000 MB
Gemma 4 31B (dense)      |  31B   |     0      |      0 MB  (no MTP support)
Gemma 4 26B-A4B (MoE)    | 3.8B   |     0      |      0 MB  (no MTP support)
```

**NVFP4 models do NOT support MTP.** If the quantization is NVFP4, set mtp_total_mb = 0 regardless of config.

### 6. CUDA Context and Graph Capture

Every vLLM process creates a CUDA context and captures CUDA graphs at startup. This is a fixed overhead per process.

```
cuda_context_mb = 1,500    (CUDA context creation)
cuda_graphs_mb  = 1,500    (graph capture for decode batch sizes [1,2,4,8])

cuda_total_mb = cuda_context_mb + cuda_graphs_mb = 3,000 MB (3 GB)
```

This cost applies per vLLM instance. Two vLLM processes = 6 GB in CUDA contexts.

For embedding/reranker models that use `--enforce-eager` (no graph capture):

```
cuda_total_mb = 1,500   (context only, no graphs)
```

### 7. Off-Budget Allocations

These are allocations that happen outside vLLM's managed memory pool: intermediate activation tensors during prefill, FlashInfer/Triton JIT kernel allocations, PyTorch CUDA allocator fragmentation, and temporary buffers.

```
For models with max_num_batched_tokens >= 16384:
  off_budget_mb = 3,000   (3 GB)

For models with max_num_batched_tokens >= 32768:
  off_budget_mb = 4,000   (4 GB — larger prefill chunks = bigger activation tensors)

For small models (embedding, reranker):
  off_budget_mb = 500
```

---

## The Complete Formula

```
total_needed_mb = weights_mb
                + kv_cache_mb
                + gdn_state_mb         (hybrid models only, typically ~50-100 MB)
                + prefix_cache_mb
                + mtp_total_mb          (0 if no MTP or NVFP4)
                + cuda_total_mb
                + off_budget_mb
```

### Deriving gpu_memory_utilization

```
gpu_memory_utilization = total_needed_mb / TOTAL_GPU_MB

# Round UP to nearest 0.01
gpu_memory_utilization = ceil(gpu_memory_utilization × 100) / 100
```

### Deriving Docker Memory Limit

Docker memory limits on the GB10 don't fully catch CUDA allocations (unified memory bypasses cgroups for GPU allocs). But they catch CPU-side leaks and provide a last-resort safety net.

```
docker_limit_mb = total_needed_mb × 1.15   (15% headroom)
docker_limit_gb = ceil(docker_limit_mb / 1024)
```

---

## Multi-Model Validation

Before launching a new model, validate that the combined allocation fits:

```
currently_used_mb = sum of total_needed_mb for all running instances
proposed_mb = total_needed_mb for the new model

if (currently_used_mb + proposed_mb) > SAFE_USABLE_MB:
    ERROR: "Cannot fit. Needs X MB, only Y MB safely available."
    SUGGEST: models that could be stopped to make room
else:
    OK: proceed with launch
```

### Reading Current Free Memory

```bash
# Method 1: /proc/meminfo (preferred, accounts for all allocations)
awk '/MemAvailable/ {print int($2/1024)}' /proc/meminfo

# Method 2: nvidia-smi (may not reflect unified memory accurately)
nvidia-smi --query-gpu=memory.free --format=csv,noheader,nounits

# Method 3: Python
import torch
free, total = torch.cuda.mem_get_info()
free_mb = free // (1024 * 1024)
```

Use Method 1 (`/proc/meminfo MemAvailable`) as the source of truth. It correctly reflects all memory usage including CUDA allocations on unified memory.

---

## Worked Examples

### Example 1: Qwen3.6-35B-A3B FP8, 2 agents, 262K context, MTP=2

```
weights_mb        = 35 × 1.0 × 1024              = 35,840 MB
kv_cache_mb       = (10,240 × 262,144 × 2) / 1M  =  5,120 MB
gdn_state_mb      = 50 × 2                        =    100 MB
prefix_cache_mb   =                                =  1,024 MB
mtp_total_mb      = (3B active, 2 tokens)          =  2,000 MB
cuda_total_mb     =                                =  3,000 MB
off_budget_mb     = (batched 32768)                =  4,000 MB
────────────────────────────────────────────────────────────────
total_needed_mb   =                                = 51,084 MB

gpu_memory_utilization = 51,084 / 121,856 = 0.42
docker_limit_mb        = 51,084 × 1.15   = 58,747 MB → 58g
```

### Example 2: Qwen3.6-27B FP8, 1 agent, 131K context, MTP=3

```
weights_mb        = 27 × 1.0 × 1024              = 27,648 MB
kv_cache_mb       = (16,384 × 131,072 × 1) / 1M  =  2,048 MB
gdn_state_mb      = 50 × 1                        =     50 MB
prefix_cache_mb   =                                =  1,024 MB
mtp_total_mb      = (27B active, 3 tokens)         =  7,500 MB
cuda_total_mb     =                                =  3,000 MB
off_budget_mb     = (batched 16384)                =  3,000 MB
────────────────────────────────────────────────────────────────
total_needed_mb   =                                = 44,270 MB

gpu_memory_utilization = 44,270 / 121,856 = 0.37
docker_limit_mb        = 44,270 × 1.15   = 50,911 MB → 50g
```

### Example 3: Gemma 4 26B-A4B FP8, 1 agent, 131K context, no MTP

```
weights_mb        = 26 × 1.0 × 1024                = 26,624 MB
kv_cache_mb       = (106,496 × 131,072 × 1) / 1M   = 13,312 MB
gdn_state_mb      = 0                               =      0 MB
prefix_cache_mb   =                                  =  1,024 MB
mtp_total_mb      = 0                                =      0 MB
cuda_total_mb     =                                  =  3,000 MB
off_budget_mb     =                                  =  3,000 MB
────────────────────────────────────────────────────────────────
total_needed_mb   =                                  = 46,960 MB

gpu_memory_utilization = 46,960 / 121,856 = 0.39
docker_limit_mb        = 46,960 × 1.15   = 54,004 MB → 53g
```

### Example 4: Multi-model validation

Running Example 1 + Example 2 + Embed + Rerank simultaneously:

```
Model 1 (35B FP8):     51,084 MB
Model 2 (27B FP8):     44,270 MB
Embedding (0.6B):       3,200 MB
Reranker (0.6B):        3,200 MB
─────────────────────────────────
Total:                101,754 MB
Safe usable:          105,912 MB
Headroom:               4,158 MB  (3.9 GB)

⚠️ Tight but fits. earlyoom at 3% provides protection.
```

---

## Model Profile Data Structure

Each model should have a profile containing the architecture-specific constants needed for calculation:

```yaml
model_profiles:
  qwen3.6-35b-a3b:
    total_params_b: 35
    active_params_b: 3
    architecture: moe_hybrid        # moe_hybrid | dense_hybrid | moe | dense
    total_layers: 40
    attention_layers: 10
    gdn_layers: 30
    num_kv_heads: 2
    head_dim: 256
    supports_mtp: true
    supports_vision: true
    default_context: 262144
    max_context: 262144
    quantizations:
      bf16:
        bytes_per_param: 2.0
        hf_repo: "Qwen/Qwen3.6-35B-A3B"
        mtp_compatible: true
      fp8:
        bytes_per_param: 1.0
        hf_repo: "Qwen/Qwen3.6-35B-A3B-FP8"
        mtp_compatible: true
      nvfp4:
        bytes_per_param: 0.5
        hf_repo: "RedHatAI/Qwen3.6-35B-A3B-NVFP4"
        mtp_compatible: false       # MTP not supported on NVFP4

  qwen3.6-27b:
    total_params_b: 27
    active_params_b: 27             # dense model — all params active
    architecture: dense_hybrid
    total_layers: 48
    attention_layers: 16
    gdn_layers: 32
    num_kv_heads: 4
    head_dim: 128
    supports_mtp: true
    supports_vision: true
    default_context: 262144
    max_context: 262144
    quantizations:
      bf16:
        bytes_per_param: 2.0
        hf_repo: "Qwen/Qwen3.6-27B"
        mtp_compatible: true
      fp8:
        bytes_per_param: 1.0
        hf_repo: "Qwen/Qwen3.6-27B-FP8"
        mtp_compatible: true
      nvfp4:
        bytes_per_param: 0.5
        hf_repo: "sakamakismile/Qwen3.6-27B-NVFP4"
        mtp_compatible: false

  gemma-4-31b:
    total_params_b: 31
    active_params_b: 31             # dense model
    architecture: dense
    total_layers: 30
    attention_layers: 30            # ALL layers are attention
    gdn_layers: 0
    num_kv_heads: 8
    head_dim: 256
    supports_mtp: false             # Gemma does not support MTP
    supports_vision: true
    default_context: 262144
    max_context: 262144
    quantizations:
      bf16:
        bytes_per_param: 2.0
        hf_repo: "google/gemma-4-31B-it"
        mtp_compatible: false
      fp8:
        bytes_per_param: 1.0
        hf_repo: "RedHatAI/gemma-4-31B-it-FP8-Dynamic"
        mtp_compatible: false
      nvfp4:
        bytes_per_param: 0.5
        hf_repo: "nvidia/Gemma-4-31B-IT-NVFP4"
        mtp_compatible: false

  gemma-4-26b-a4b:
    total_params_b: 26
    active_params_b: 3.8
    architecture: moe
    total_layers: 26
    attention_layers: 26            # ALL layers are attention
    gdn_layers: 0
    num_kv_heads: 8
    head_dim: 256
    supports_mtp: false
    supports_vision: true
    default_context: 262144
    max_context: 262144
    quantizations:
      bf16:
        bytes_per_param: 2.0
        hf_repo: "google/gemma-4-26B-A4B-it"
        mtp_compatible: false
      fp8:
        bytes_per_param: 1.0
        hf_repo: "RedHatAI/gemma-4-26B-A4B-it-FP8-Dynamic"
        mtp_compatible: false
      nvfp4:
        bytes_per_param: 0.5
        hf_repo: "RedHatAI/gemma-4-26B-A4B-it-NVFP4"
        mtp_compatible: false

  qwen3.5-35b-a3b:
    total_params_b: 35
    active_params_b: 3
    architecture: moe_hybrid
    total_layers: 40
    attention_layers: 10
    gdn_layers: 30
    num_kv_heads: 2
    head_dim: 256
    supports_mtp: true
    supports_vision: true
    default_context: 262144
    max_context: 262144
    quantizations:
      bf16:
        bytes_per_param: 2.0
        hf_repo: "Qwen/Qwen3.5-35B-A3B"
        mtp_compatible: true

  qwen3-coder-next:
    total_params_b: 80
    active_params_b: 3
    architecture: moe_hybrid
    total_layers: 48
    attention_layers: 10
    gdn_layers: 38
    num_kv_heads: 2
    head_dim: 256
    supports_mtp: false             # Not tested/available for this model
    supports_vision: false          # Text only
    default_context: 262144
    max_context: 262144
    quantizations:
      fp8:
        bytes_per_param: 1.0
        hf_repo: "Qwen/Qwen3-Coder-Next-FP8"
        mtp_compatible: false

  qwen3-embedding-0.6b:
    total_params_b: 0.6
    active_params_b: 0.6
    architecture: encoder
    total_layers: 0
    attention_layers: 0
    gdn_layers: 0
    num_kv_heads: 0
    head_dim: 0
    supports_mtp: false
    supports_vision: false
    default_context: 8192
    max_context: 8192
    quantizations:
      fp16:
        bytes_per_param: 2.0
        hf_repo: "Qwen/Qwen3-Embedding-0.6B"
        mtp_compatible: false

  qwen3-reranker-0.6b:
    total_params_b: 0.6
    active_params_b: 0.6
    architecture: encoder
    total_layers: 0
    attention_layers: 0
    gdn_layers: 0
    num_kv_heads: 0
    head_dim: 0
    supports_mtp: false
    supports_vision: false
    default_context: 8192
    max_context: 8192
    quantizations:
      fp16:
        bytes_per_param: 2.0
        hf_repo: "Qwen/Qwen3-Reranker-0.6B"
        mtp_compatible: false
```

---

## Implementation Pseudocode

```python
def calculate_memory(profile, quant, context_len, num_agents, mtp_tokens):
    q = profile.quantizations[quant]
    
    # 1. Weights
    weights_mb = profile.total_params_b * q.bytes_per_param * 1024
    
    # 2. KV Cache
    if profile.architecture == 'encoder':
        kv_cache_mb = 0
    else:
        kv_per_token = (2 * profile.num_kv_heads * profile.head_dim 
                        * profile.attention_layers * 1)  # 1 byte for FP8 KV
        kv_cache_mb = (kv_per_token * context_len * num_agents) / (1024 * 1024)
    
    # 3. GDN State
    gdn_mb = 50 * num_agents if profile.gdn_layers > 0 else 0
    
    # 4. Prefix Cache
    if profile.architecture == 'encoder':
        prefix_mb = 0
    elif num_agents >= 2:
        prefix_mb = 2048  # 2 GB for heavy multi-agent
    else:
        prefix_mb = 1024  # 1 GB standard
    
    # 5. MTP
    if mtp_tokens > 0 and q.mtp_compatible:
        active = profile.active_params_b
        mtp_head = active * 500
        mtp_draft = active * 333 * mtp_tokens
        mtp_verify = active * 167 * (mtp_tokens + 1)
        mtp_mb = mtp_head + mtp_draft + mtp_verify
    else:
        mtp_mb = 0
    
    # 6. CUDA Context + Graphs
    if profile.architecture == 'encoder':
        cuda_mb = 1500  # No graph capture for encoders
    else:
        cuda_mb = 3000
    
    # 7. Off-budget
    if profile.architecture == 'encoder':
        off_budget_mb = 500
    elif context_len > 65536:
        off_budget_mb = 4000  # Large context = bigger activation tensors
    else:
        off_budget_mb = 3000
    
    total = weights_mb + kv_cache_mb + gdn_mb + prefix_mb + mtp_mb + cuda_mb + off_budget_mb
    
    return {
        'total_mb': int(total),
        'gpu_memory_utilization': math.ceil(total / TOTAL_GPU_MB * 100) / 100,
        'docker_limit_gb': math.ceil(total * 1.15 / 1024),
        'breakdown': {
            'weights_mb': int(weights_mb),
            'kv_cache_mb': int(kv_cache_mb),
            'gdn_state_mb': int(gdn_mb),
            'prefix_cache_mb': int(prefix_mb),
            'mtp_mb': int(mtp_mb),
            'cuda_mb': int(cuda_mb),
            'off_budget_mb': int(off_budget_mb),
        }
    }


def validate_multi_model(models_to_run):
    """
    models_to_run: list of calculate_memory() results
    Returns: validation result with headroom info
    """
    total_needed = sum(m['total_mb'] for m in models_to_run)
    headroom = SAFE_USABLE_MB - total_needed
    
    return {
        'fits': headroom >= 0,
        'total_needed_mb': total_needed,
        'safe_usable_mb': SAFE_USABLE_MB,
        'headroom_mb': headroom,
        'headroom_gb': round(headroom / 1024, 1),
        'risk': 'safe' if headroom > 8192 
                else 'ok' if headroom > 4096
                else 'tight' if headroom > 0
                else 'does_not_fit'
    }


def can_fit_dynamic(profile, quant, context_len, num_agents, mtp_tokens):
    """
    Check if a model can fit given CURRENT free memory.
    Use this when starting a model while others are already running.
    """
    result = calculate_memory(profile, quant, context_len, num_agents, mtp_tokens)
    
    # Read current free memory from /proc/meminfo
    free_mb = read_memavailable_mb()
    safety_margin = 5120  # 5 GB
    
    available = free_mb - safety_margin
    fits = result['total_mb'] <= available
    
    return {
        'fits': fits,
        'needed_mb': result['total_mb'],
        'available_mb': available,
        'free_mb': free_mb,
        'headroom_mb': available - result['total_mb'] if fits else 0,
        'gpu_memory_utilization': result['gpu_memory_utilization'],
        'docker_limit_gb': result['docker_limit_gb'],
    }
```

---

## Edge Cases and Warnings

1. **NVFP4 + MTP = invalid.** Always set mtp_tokens=0 for NVFP4 quantizations regardless of user request. Log a warning.

2. **Gemma models have no MTP.** supports_mtp=false. Ignore any MTP configuration for Gemma models.

3. **Encoder models (embedding, reranker) have no KV cache.** kv_cache_mb=0, no MTP, reduced CUDA overhead.

4. **Start order matters.** The first model started gets clean memory. Subsequent models may see phantom memory from other CUDA contexts. Add 2-5% to gpu_memory_utilization for the second and third instances as a safety buffer.

5. **Context length should not exceed max_context.** Validate that requested context_len <= profile.max_context.

6. **Gemma 4 KV cache is 10x larger than Qwen hybrid.** Always flag when Gemma models are being paired with others at high context lengths. Suggest reducing Gemma's context before reducing other models.

7. **Docker memory limits don't fully protect against GPU OOM on unified memory.** They are a safety net, not a guarantee. The real protection is correct gpu_memory_utilization sizing + earlyoom + swapoff.

8. **vLLM pre-allocates at startup.** Memory is grabbed immediately, not on demand. An idle model uses the same memory as a fully loaded one.