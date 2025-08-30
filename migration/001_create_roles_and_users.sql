-- Migration: create roles table and ensure users.role_id FK
-- idempotent
CREATE TABLE IF NOT EXISTS roles (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    name VARCHAR(32) UNIQUE NOT NULL,
    description VARCHAR(255)
);

-- Ensure columns on users table exist (GORM may have created, do best-effort)
ALTER TABLE IF EXISTS users
    ADD COLUMN IF NOT EXISTS role_id integer;

-- Add FK constraint if not exists
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints tc
        JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name
        WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name = 'users' AND kcu.column_name = 'role_id'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_role_id_fkey FOREIGN KEY (role_id) REFERENCES roles(id) ON UPDATE CASCADE ON DELETE SET NULL;
    END IF;
EXCEPTION WHEN others THEN
    -- ignore
END$$;

-- Add unique index on catatan_keuangans (if table exists)
-- Create the unique index if the table exists. CREATE INDEX CONCURRENTLY cannot run inside a DO block,
-- so perform a safe check and normal CREATE which will hold a brief DDL lock if needed.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'catatan_keuangans') THEN
        IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename='catatan_keuangans' AND indexname='unique_user_filename') THEN
            EXECUTE 'CREATE UNIQUE INDEX IF NOT EXISTS unique_user_filename ON catatan_keuangans (user_id, file_name)';
        END IF;
    END IF;
END$$;
