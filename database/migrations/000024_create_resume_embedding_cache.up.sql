CREATE TABLE IF NOT EXISTS resume_embedding_cache (
    resume_hash TEXT PRIMARY KEY,
    embedding halfvec(3072) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
