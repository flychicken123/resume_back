@echo off
echo Cleaning Git history to remove secrets...
echo.

echo Removing file from all commits...
git filter-branch --force --index-filter "git rm --cached --ignore-unmatch DATABASE_TROUBLESHOOTING.md" --prune-empty --tag-name-filter cat -- --all

echo.
echo Force pushing to overwrite remote history...
echo WARNING: This will overwrite the remote repository history!
pause

git push origin main --force

echo.
echo Git history cleaned successfully!
echo Remember to change your database password!
pause 