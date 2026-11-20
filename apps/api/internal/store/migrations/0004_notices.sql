-- The notices the API e-mails on its own every hour (internal/notices):
-- which ones went out, so that each goes once, whatever the restarts or the
-- instances. A row is written only after the mail service accepted the
-- message. Never the recipients nor the text.

-- An active certificate about to expire (30, 7 and 1 days before its
-- not_after) or expired while still active: one row per certificate and
-- threshold notified. It goes with the certificate.
CREATE TABLE certificate_notices (
	certificate_id uuid NOT NULL REFERENCES certificates (id) ON DELETE CASCADE,
	threshold text NOT NULL CHECK (threshold IN ('30d', '7d', '1d', 'expired')),
	sent_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (certificate_id, threshold)
);

-- The hourly job reads the active certificates that expire soon.
CREATE INDEX certificates_active_not_after ON certificates (not_after) WHERE active;

-- What a space signed in an environment in a month reaching a point of its
-- limits: 80 % of its quota, its whole quota (when signing goes on past it)
-- or its cap. One row per space, environment, month, threshold and the
-- limit it was measured against (`limit_value`: a plan that changes in the
-- month is measured again). The job deletes the rows of the months before
-- the last one.
CREATE TABLE usage_notices (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	environment text NOT NULL CHECK (environment IN ('test', 'prod')),
	month text NOT NULL,
	threshold text NOT NULL CHECK (threshold IN ('quota-80', 'quota', 'cap')),
	limit_value integer NOT NULL,
	sent_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (workspace_id, environment, month, threshold, limit_value)
);
CREATE INDEX usage_notices_month ON usage_notices (month);
