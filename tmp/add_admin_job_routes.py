from pathlib import Path
text = Path("main.go").read_text()
marker = "\t\tadmin := protected.Group(\"/admin\")\n\t\tadmin.Use(middleware.RequireAdmin())\n\t\t{\n\t\t\tadmin.GET(\"/memberships/users\", adminController.ListUsers)\n\t\t\tadmin.PUT(\"/memberships/users/:id\", adminController.UpdateUserMembership)\n\t\t}\n"
if marker not in text:
    raise SystemExit('admin block marker not found')
injection = "\t\tadmin := protected.Group(\"/admin\")\n\t\tadmin.Use(middleware.RequireAdmin())\n\t\t{\n\t\t\tadmin.GET(\"/memberships/users\", adminController.ListUsers)\n\t\t\tadmin.PUT(\"/memberships/users/:id\", adminController.UpdateUserMembership)\n\t\t\tadmin.GET(\"/jobs/companies\", jobsController.ListCompanies)\n\t\t\tadmin.POST(\"/jobs/companies\", jobsController.CreateCompany)\n\t\t\tadmin.POST(\"/jobs/companies/:id/sync\", jobsController.TriggerSync)\n\t\t}\n"
text = text.replace(marker, injection, 1)
Path("main.go").write_text(text)
