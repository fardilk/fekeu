This folder previously contained a small sqlite inspection tool (cmd_inspect_fk.go).
The project now uses Postgres and we removed the sqlite driver from the main module.

If you need the inspection tool, restore it and run `go get gorm.io/driver/sqlite` in this module,
or run it from a separate module that adds the sqlite driver.

Original code archived here for reference.
