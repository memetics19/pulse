-- Optional HTTP monitor options. Every default keeps the old behaviour: a GET
-- with no extra headers, no body and no auth, normal TLS verification,
-- redirects followed, any 2xx accepted, and a "contains" keyword check.
-- tls_verify and follow_redirects are NULL by default and NULL means true.
ALTER TABLE monitors ADD COLUMN method TEXT NOT NULL DEFAULT 'GET';
-- headers is a JSON object of header name to value, or '' for none.
ALTER TABLE monitors ADD COLUMN headers TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN body TEXT NOT NULL DEFAULT '';
-- auth_type is '', 'basic' or 'bearer'. auth_secret holds the basic password
-- or the bearer token. The API never returns it.
ALTER TABLE monitors ADD COLUMN auth_type TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN auth_username TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN auth_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN tls_verify INTEGER;
ALTER TABLE monitors ADD COLUMN ca_pem TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN follow_redirects INTEGER;
-- accepted_statuses is a comma-separated list such as '200-299,301'.
ALTER TABLE monitors ADD COLUMN accepted_statuses TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN keyword_mode TEXT NOT NULL DEFAULT 'contains';
