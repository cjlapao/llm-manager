ALTER TABLE models ADD COLUMN max_num_seqs INTEGER;
ALTER TABLE models ADD COLUMN max_num_batched_tokens INTEGER;
ALTER TABLE models ADD COLUMN speculative_decoding TEXT;
ALTER TABLE models ADD COLUMN num_speculative_tokens INTEGER;
