CREATE TABLE job_applications (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_posting_id BIGINT REFERENCES job_postings(id) ON DELETE SET NULL,
    resume_job_match_id BIGINT REFERENCES resume_job_matches(id) ON DELETE SET NULL,

    -- Denormalized (survives if job_posting deleted)
    job_title TEXT NOT NULL,
    company_name TEXT NOT NULL DEFAULT '',
    job_url TEXT NOT NULL DEFAULT '',
    job_location TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL DEFAULT 'applied',
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resume_hash TEXT,
    cover_letter_used BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT,
    salary_offered NUMERIC,
    salary_currency TEXT,

    status_updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_job_applications_user_status ON job_applications(user_id, status);
CREATE INDEX idx_job_applications_user_created ON job_applications(user_id, created_at DESC);
CREATE UNIQUE INDEX ux_job_applications_user_job ON job_applications(user_id, job_posting_id) WHERE job_posting_id IS NOT NULL;

CREATE TABLE job_application_status_history (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES job_applications(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status TEXT NOT NULL,
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    note TEXT
);

CREATE INDEX idx_app_status_history_app ON job_application_status_history(application_id, changed_at DESC);
