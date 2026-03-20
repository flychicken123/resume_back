CREATE TABLE dismissed_job_matches (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_posting_id BIGINT NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    dismiss_reason TEXT,
    dismissed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, job_posting_id)
);

CREATE INDEX idx_dismissed_job_matches_user ON dismissed_job_matches (user_id);
