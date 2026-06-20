package service

// StartOverrides holds optional CLI flags that override auto-calculated
// vLLM parameters at start time. Zero values mean "use auto-calc".
// Pointer fields (nil = not set) allow distinguishing between "not provided"
// and "explicitly zero".
type StartOverrides struct {
	MaxModelLen          int      // overrides --max-model-len
	MaxNumSeqs           int      // overrides --max-num-seqs
	MaxNumBatchedTokens  int      // overrides --max-num-batched-tokens
	GPUMemoryUtil        *float64 // overrides gpu_memory_utilization (bypasses calc)
	SpeculativeDecoding  *string  // --speculative-decoding (e.g., "mtp", "dflash")
	NumSpeculativeTokens *int     // --speculative-tokens count
	SpeculativeModel     *string  // --speculative-model (draft model for dflash etc.)
}
