CREATE EXTENSION IF NOT EXISTS vector;
ALTER TABLE job_postings ADD COLUMN IF NOT EXISTS embedding halfvec(3072);
CREATE INDEX IF NOT EXISTS idx_job_postings_embedding_cosine
    ON job_postings USING hnsw (embedding halfvec_cosine_ops)
    WITH (m = 24, ef_construction = 100);
