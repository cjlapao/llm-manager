package service

// StartOverrides holds optional CLI flags that override auto-calculated
// vLLM parameters at start time. Zero values mean "use auto-calc".
type StartOverrides struct {
	MaxModelLen         int // overrides --max-model-len
	MaxNumSeqs          int // overrides --max-num-seqs
	MaxNumBatchedTokens int // overrides --max-num-batched-tokens
}
