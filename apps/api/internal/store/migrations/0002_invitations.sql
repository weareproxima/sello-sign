-- Invitations to a space by e-mail (self-hosted spaces and the cloud's
-- spaces that aren't of an organization of Sello's IdP: in those, the
-- members are the seats billing pushes). The token goes in the e-mail's
-- link; only its SHA-256 is kept, as with the API keys.
CREATE TABLE invitations (
	id uuid PRIMARY KEY,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	-- Trimmed and in lower case: what's compared with the token's e-mail.
	email text NOT NULL,
	role text NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
	hash bytea NOT NULL UNIQUE,
	-- The panel user who invited.
	invited_by text NOT NULL,
	expires_at timestamptz NOT NULL,
	accepted_at timestamptz,
	-- The panel user who accepted it.
	accepted_by text,
	revoked_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);
-- One open invitation (neither accepted nor revoked, expired or not) per
-- address in a space; it also serves the space's list.
CREATE UNIQUE INDEX invitations_open ON invitations (workspace_id, email)
	WHERE accepted_at IS NULL AND revoked_at IS NULL;
