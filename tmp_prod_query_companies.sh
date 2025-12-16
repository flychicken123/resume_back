set -euo pipefail
cid=ai-resume-backend-backend-1
DB_HOST=$(sudo docker exec "$cid" printenv DB_HOST)
DB_PORT=$(sudo docker exec "$cid" printenv DB_PORT)
DB_USER=$(sudo docker exec "$cid" printenv DB_USER)
DB_PASSWORD=$(sudo docker exec "$cid" printenv DB_PASSWORD)
DB_NAME=$(sudo docker exec "$cid" printenv DB_NAME)

# Query company status
sudo docker run --rm -e PGPASSWORD="$DB_PASSWORD" postgres:16-alpine psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT id,name,ats_provider,careers_url,is_active,sync_failure_count,last_synced_at,last_sync_status,COALESCE(left(last_sync_error,120), '') AS err FROM job_companies WHERE id IN (8435,8436) ORDER BY id;"ca