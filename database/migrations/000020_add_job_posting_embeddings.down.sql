DROP INDEX IF EXISTS idx_job_postings_embedding_cosine;
ALTER TABLE job_postings DROP COLUMN IF EXISTS embedding;
