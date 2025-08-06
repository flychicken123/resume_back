# PowerShell script to clean Git history and remove secrets

Write-Host "Cleaning Git history to remove secrets..." -ForegroundColor Yellow

# Remove the file from all commits
git filter-branch --force --index-filter "git rm --cached --ignore-unmatch DATABASE_TROUBLESHOOTING.md" --prune-empty --tag-name-filter cat -- --all

# Force push to overwrite remote history
Write-Host "Force pushing to overwrite remote history..." -ForegroundColor Red
Write-Host "WARNING: This will overwrite the remote repository history!" -ForegroundColor Red
Write-Host "Press any key to continue..." -ForegroundColor Yellow
Read-Host

git push origin main --force

Write-Host "Git history cleaned successfully!" -ForegroundColor Green
Write-Host "Remember to change your database password!" -ForegroundColor Red 