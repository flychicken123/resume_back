from pathlib import Path
text = Path("main.go").read_text()
marker = "\tadminController := controllers.NewAdminController(db)\n\n\tr := gin.Default()\n"
if marker not in text:
    raise SystemExit('admin controller marker not found')
injection = "\tadminController := controllers.NewAdminController(db)\n\n\tctx, cancel := context.WithCancel(context.Background())\n\tdefer cancel()\n\tjobsService.StartScheduler(ctx, 30*time.Minute)\n\tgo func() {\n\t\tif err := jobsService.SyncAllCompanies(ctx); err != nil {\n\t\t\tlogger.Warn(\"initial job sync failed\", map[string]interface{}{\"error\": err.Error()})\n\t\t}\n\t}()\n\n\tr := gin.Default()\n"
text = text.replace(marker, injection, 1)
Path("main.go").write_text(text)
