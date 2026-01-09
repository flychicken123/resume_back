-- Down migration: 000006_update_exit_events_add_context

ALTER TABLE exit_events DROP COLUMN IF EXISTS referrer;
ALTER TABLE exit_events DROP COLUMN IF EXISTS last_step_delta_ms;
ALTER TABLE exit_events DROP COLUMN IF EXISTS page_duration_ms;
ALTER TABLE exit_events DROP COLUMN IF EXISTS session_duration_ms;
ALTER TABLE exit_events DROP COLUMN IF EXISTS previous_page_path;
