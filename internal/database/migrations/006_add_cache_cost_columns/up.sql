ALTER TABLE models ADD COLUMN cache_creation_input_token_cost REAL DEFAULT 0;
ALTER TABLE models ADD COLUMN cache_read_input_token_cost REAL DEFAULT 0;
