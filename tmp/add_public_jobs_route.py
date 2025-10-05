from pathlib import Path
text = Path("main.go").read_text()
marker = "\t\tapi.POST(\"/auth/logout\", handlers.LogoutUser())\n\t}\n\n\t// Public routes (no auth required) - keep using handlers for now\n"
if marker not in text:
    raise SystemExit('api logout marker not found')
injection = "\t\tapi.POST(\"/auth/logout\", handlers.LogoutUser())\n\t\tapi.GET(\"/jobs\", jobsController.ListJobs)\n\t}\n\n\t// Public routes (no auth required) - keep using handlers for now\n"
text = text.replace(marker, injection, 1)
Path("main.go").write_text(text)
