package main

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// RunInspectFKs connects to Postgres using dsn and prints foreign key constraints.
func RunInspectFKs(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("dsn is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT
		  con.oid::regclass::text AS constraint_name,
		  rel.relname AS table_name,
		  array_agg(att.attname ORDER BY u.attnum) AS src_columns,
		  confrel.relname AS referenced_table,
		  array_agg(att2.attname ORDER BY u.confkey) AS ref_columns,
		  pg_get_constraintdef(con.oid) AS definition
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_class confrel ON confrel.oid = con.confrelid
		JOIN unnest(con.conkey) WITH ORDINALITY AS u(attnum, ord) ON true
		JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = u.attnum
		LEFT JOIN unnest(con.confkey) WITH ORDINALITY AS v(confkey, ord2) ON v.ord2 = u.ord
		LEFT JOIN pg_attribute att2 ON att2.attrelid = con.confrelid AND att2.attnum = v.confkey
		WHERE con.contype = 'f'
		GROUP BY con.oid, rel.relname, confrel.relname
		ORDER BY rel.relname, constraint_name;
	`)
	if err != nil {
		return fmt.Errorf("query constraints: %w", err)
	}
	defer rows.Close()

	fmt.Println("Foreign keys:")
	for rows.Next() {
		var cname, table, reftable, def string
		var srcCols, refCols sql.NullString
		if err := rows.Scan(&cname, &table, &srcCols, &reftable, &refCols, &def); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		fmt.Printf("- %s: %s(%s) -> %s(%s)\n    def: %s\n", cname, table, nullStringToStr(srcCols), reftable, nullStringToStr(refCols), def)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows err: %w", err)
	}
	return nil
}

func nullStringToStr(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}
