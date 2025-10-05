from pathlib import Path
text = Path("main.go").read_text()
marker = "\tfeedbackModel := models.NewFeedbackModel(db)\n\n\t// Initialize services\n"
if marker not in text:
    raise SystemExit('marker not found')
injection = "\tfeedbackModel := models.NewFeedbackModel(db)\n\n\tjobCompanyModel := models.NewJobCompanyModel(db)\n\tjobPostingModel := models.NewJobPostingModel(db)\n\tjobSyncModel := models.NewJobSyncRunModel(db)\n\n\t// Initialize services\n"
text = text.replace(marker, injection, 1)
Path("main.go").write_text(text)
