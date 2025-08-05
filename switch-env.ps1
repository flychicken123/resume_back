param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("local", "prod")]
    [string]$Environment
)

Write-Host "Switching to $Environment environment..."

if ($Environment -eq "local") {
    Copy-Item .env.local .env -Force
    Write-Host "✅ Switched to LOCAL environment (localhost PostgreSQL)"
    Write-Host "Database: localhost:5432"
    Write-Host "User: postgres"
    Write-Host "Database: airesume"
    Write-Host "Password: admin"
} elseif ($Environment -eq "prod") {
    Write-Host "⚠️  PROD environment uses GitHub Secrets directly"
    Write-Host "The pipeline automatically creates .env from GitHub Secrets"
    Write-Host "This script is only for local development testing"
    Write-Host ""
    Write-Host "For production deployment:"
    Write-Host "- Push to main branch"
    Write-Host "- GitHub Actions will deploy to EC2"
    Write-Host "- Environment variables come from GitHub Secrets (prod environment)"
    return
}

Write-Host "Environment variables updated. Restart the application to apply changes." 