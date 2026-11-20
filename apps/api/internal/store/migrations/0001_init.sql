-- Sello Sign's domain. The panel's better-auth tables live in the schema
-- `auth` of the same database and are never touched here.

-- A space: Sign's customer. In the cloud, the space of one organization of
-- Sello's IdP (sello_workspace_id), whose plan and members billing pushes.
CREATE TABLE workspaces (
	id uuid PRIMARY KEY,
	name text NOT NULL,
	default_country text,
	sello_workspace_id text UNIQUE,
	-- The cloud's plan (free, pro_starter…); NULL in a self-hosted install.
	plan text,
	-- The plan's limits; NULL is none.
	max_organizations integer,
	max_prod_signatures integer,
	max_test_signatures integer,
	max_members integer,
	requests_per_minute integer,
	retention_days integer,
	prod_hard_cap integer,
	test_hard_cap integer,
	subscription_status text,
	subscription_interval text,
	subscription_period_end timestamptz,
	subscription_cancel_at_period_end boolean NOT NULL DEFAULT false,
	-- The last state billing pushed that was applied.
	billing_version bigint NOT NULL DEFAULT 0,
	-- Sello deleted the organization: the space stops signing.
	billing_closed_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);

-- The people of a space, by the panel's user id (the token's `sub`).
CREATE TABLE workspace_members (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	user_id text NOT NULL,
	role text NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user ON workspace_members (user_id);

-- In the cloud, the members billing pushes, by the `sub` of their Sello
-- account, in seat order: the first max_members of them are members.
CREATE TABLE billing_members (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	account text NOT NULL,
	role text NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
	position integer NOT NULL,
	PRIMARY KEY (workspace_id, account)
);
CREATE INDEX billing_members_account ON billing_members (account);

-- Who the panel's users are, as their last token said: to name the members
-- of a space. Never a credential.
CREATE TABLE users (
	id text PRIMARY KEY,
	email text NOT NULL DEFAULT '',
	name text NOT NULL DEFAULT '',
	sello_sub text,
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_sello_sub ON users (sello_sub) WHERE sello_sub IS NOT NULL;

-- A taxpayer: it lives in one space, and may name its organization at
-- Sello's IdP.
CREATE TABLE organizations (
	id uuid PRIMARY KEY,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
	name text NOT NULL,
	tax_id text NOT NULL,
	-- tax_id without separators, in upper case: what's compared.
	tax_id_key text NOT NULL,
	country text NOT NULL,
	contact_email text,
	sello_workspace_id text UNIQUE,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (workspace_id, country, tax_id_key)
);
CREATE INDEX organizations_tax_id ON organizations (tax_id_key);

-- A certificate: its material (the key and the certificates, as PEM)
-- sealed with SIGN_SECRETS_KEY, and what it says of itself.
CREATE TABLE certificates (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
	environment text NOT NULL CHECK (environment IN ('test', 'prod')),
	material bytea NOT NULL,
	tax_id text,
	subject text NOT NULL DEFAULT '',
	issuer text NOT NULL DEFAULT '',
	serial text NOT NULL DEFAULT '',
	not_before timestamptz,
	not_after timestamptz,
	active boolean NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX certificates_organization ON certificates (organization_id, environment);
CREATE UNIQUE INDEX certificates_one_active ON certificates (organization_id, environment) WHERE active;

-- A delegation lets a space use an organization of another.
CREATE TABLE delegations (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	scopes text[] NOT NULL,
	-- Kept by Sello's platform space when a company took the organization:
	-- the owner can't remove it.
	locked boolean NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (organization_id, workspace_id)
);
CREATE INDEX delegations_workspace ON delegations (workspace_id);

CREATE TABLE api_keys (
	id uuid PRIMARY KEY,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	organization_id uuid REFERENCES organizations (id) ON DELETE CASCADE,
	environment text NOT NULL CHECK (environment IN ('test', 'prod')),
	name text NOT NULL,
	scopes text[] NOT NULL,
	hash bytea NOT NULL UNIQUE,
	prefix text NOT NULL,
	created_by text,
	last_used_at timestamptz,
	revoked_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_keys_workspace ON api_keys (workspace_id, id DESC) WHERE revoked_at IS NULL;

-- What was signed, never the document. Without foreign keys to the
-- organization or the key: the log outlives them.
CREATE TABLE signing_log (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	signed_by_workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	api_key_id uuid,
	user_id text,
	environment text NOT NULL CHECK (environment IN ('test', 'prod')),
	document_id text NOT NULL DEFAULT '',
	document_type text NOT NULL DEFAULT '',
	duration_ms integer NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX signing_log_signer ON signing_log (signed_by_workspace_id, id DESC);
CREATE INDEX signing_log_organization ON signing_log (organization_id, id DESC);

-- What each space signed, by organization, environment and month (UTC,
-- 'YYYY-MM'). Kept when an organization goes: it's billed.
CREATE TABLE usage_counters (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	organization_id uuid NOT NULL,
	environment text NOT NULL CHECK (environment IN ('test', 'prod')),
	month text NOT NULL,
	count bigint NOT NULL DEFAULT 0,
	PRIMARY KEY (workspace_id, organization_id, environment, month)
);
CREATE INDEX usage_counters_month ON usage_counters (month);
