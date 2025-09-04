Cors Configuration
==================

Environment variable: ALLOWED_ORIGINS (comma separated list)

Default (if unset):
  http://localhost:3000,http://localhost:3001,http://localhost:3002,http://localhost:3003

Example .env:
  ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173

Behavior:
  - Reflects the matching Origin in Access-Control-Allow-Origin (no wildcard when credentials allowed).
  - Sends: Access-Control-Allow-Methods: GET,POST,PUT,PATCH,DELETE,OPTIONS
           Access-Control-Allow-Headers: Authorization,Content-Type,Accept,Origin,X-Requested-With
           Access-Control-Allow-Credentials: true
           Access-Control-Max-Age: 43200 (12h)
  - OPTIONS preflight returns 204 (No Content).

Add ports: just extend ALLOWED_ORIGINS.

Frontend fetch example (with credentials if needed):
fetch('http://localhost:8081/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username, password })
});

Security Note:
  Keep production origins tight (no wildcard). Set explicit HTTPS origins in production .env.
