CREATE TABLE ai_benchmark_results (
    id BIGSERIAL PRIMARY KEY,
    run_id TEXT NOT NULL,
    benchmark_type TEXT NOT NULL,
    entity_id BIGINT,
    field_name TEXT NOT NULL,
    ai_value TEXT,
    expected_value TEXT,
    is_correct BOOLEAN,
    score NUMERIC(3,1),
    reasoning TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_benchmark_run ON ai_benchmark_results (run_id);
CREATE INDEX idx_benchmark_type ON ai_benchmark_results (benchmark_type, created_at DESC);
