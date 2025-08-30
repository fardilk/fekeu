-- Seed roles and two users (admin and fardiluser).
-- Note: we will insert roles and then create users using the application's Register function
-- because passwords need bcrypt hashing; however we provide SQL for roles and a convenience user

-- Insert roles idempotently
INSERT INTO roles (name, description)
VALUES
  ('administrator', 'full admin'),
  ('user', 'regular user')
ON CONFLICT (name) DO NOTHING;

-- Show how to insert a user via SQL directly (not recommended because password must be hashed):
-- If you want to insert a password via SQL, compute bcrypt hash externally and insert into hashed_password column.
-- Example (placeholder):
-- INSERT INTO users (username, hashed_password, role_id, created_at, updated_at) VALUES ('admin', '\x...', (SELECT id FROM roles WHERE name='administrator'), now(), now());

-- Instead, use the app endpoints to create users (recommended):
-- POST /register {"username":"admin","password":"admin"}
-- POST /register {"username":"fardiluser","password":"user"}

-- For automation, you can run the following shell snippet to register via HTTP:
-- curl -s -X POST -H "Content-Type: application/json" -d '{"username":"admin","password":"admin"}' http://127.0.0.1:8081/register
-- curl -s -X POST -H "Content-Type: application/json" -d '{"username":"fardiluser","password":"user"}' http://127.0.0.1:8081/register
