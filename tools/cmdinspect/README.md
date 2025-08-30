# cmdinspect (archived)

This folder previously contained a small sqlite inspection tool used during development.

The main project now uses Postgres; to avoid adding the sqlite driver to the main module the tool was removed.

If you need to re-enable the inspection tool, create a separate module or add the sqlite driver:

```sh
go get gorm.io/driver/sqlite
go run ./tools/cmdinspect
```
