from pathlib import Path
text = Path("main.go").read_text()
marker = "\temailService := services.NewEmailService()\n\n\t// Initialize Stripe products (only run this once or on startup)\n"
if marker not in text:
    raise SystemExit('marker not found for services block')
injection = "\temailService := services.NewEmailService()\n\tjobsService := services.NewJobIngestionService(db, logger)\n\tjobsController := controllers.NewJobsController(jobCompanyModel, jobPostingModel, jobSyncModel, jobsService)\n\n\t// Initialize Stripe products (only run this once or on startup)\n"
text = text.replace(marker, injection, 1)
Path("main.go").write_text(text)
