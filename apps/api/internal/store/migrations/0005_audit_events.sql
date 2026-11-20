-- The audit log (docs/rebuild.md, phase 2): who changed what in a space,
-- for those who administer it (GET /workspaces/{w}/audit-events). Each
-- event is written in the transaction of its change. Kept 365 days
-- (audit.RetentionDays), purged hourly with the signing log, and deleted
-- with its space. Never a secret: no key, hash, token, certificate material
-- nor PIN; and no IP address nor user agent.
CREATE TABLE audit_events (
	-- UUIDv7: ordered by time, which the list's cursor and the purge use.
	id uuid PRIMARY KEY,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	-- Who: a person (user, by the panel's `sub`), an API key, Sello's
	-- billing, the server's console, another space (on what it did to this
	-- one's organizations or delegations) or the API itself (system).
	actor_type text NOT NULL CHECK (actor_type IN ('user', 'api_key', 'billing', 'console', 'workspace', 'system')),
	-- The person's `sub`, the key's id or the space's id.
	actor_id text,
	-- As the API knew them at that moment: a person's e-mail and name, from
	-- their token; a key's or a space's name.
	actor_email text,
	actor_name text NOT NULL DEFAULT '',
	-- 'mcp' when a key acted through the MCP server.
	via text CHECK (via IN ('mcp')),
	-- `<target>.<verb>`: audit.Actions.
	action text NOT NULL,
	target_type text NOT NULL,
	target_id text,
	-- A short name for it at that moment: the organization's, the key's,
	-- the member's or the invited e-mail.
	target_label text NOT NULL DEFAULT '',
	-- What changed (old → new, the role…).
	details jsonb NOT NULL DEFAULT '{}',
	created_at timestamptz NOT NULL
);
-- A space's list, newest first, and its filters.
CREATE INDEX audit_events_workspace ON audit_events (workspace_id, id DESC);
