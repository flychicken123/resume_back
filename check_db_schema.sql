-- Check if resume_history table exists and show its structure
SELECT 
    table_name,
    column_name,
    data_type,
    is_nullable,
    column_default
FROM information_schema.columns 
WHERE table_name = 'resume_history'
ORDER BY ordinal_position;

-- Check if there are any records in resume_history
SELECT COUNT(*) as record_count FROM resume_history;

-- Show recent records if any exist
SELECT * FROM resume_history ORDER BY generated_at DESC LIMIT 5;