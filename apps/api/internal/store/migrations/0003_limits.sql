-- The limits, applied (docs/rebuild.md, phase 2): the space's own signing
-- rate, each key's, and what a space signed in a month in one row, read on
-- every signature to stop it at its plan's cap.

-- The signing rate a space sets for itself, below its plan's ceiling
-- (requests_per_minute); NULL is the ceiling.
ALTER TABLE workspaces ADD COLUMN own_requests_per_minute integer CHECK (own_requests_per_minute > 0);

-- A key's own signing rate, on top of its space's; NULL is none.
ALTER TABLE api_keys ADD COLUMN requests_per_minute integer CHECK (requests_per_minute > 0);

-- What each space signed, by environment and month: usage_counters summed
-- over its organizations. A trigger keeps it, whoever writes the counters,
-- so it never drifts from what billing is reported.
CREATE TABLE workspace_usage (
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	environment text NOT NULL CHECK (environment IN ('test', 'prod')),
	month text NOT NULL,
	count bigint NOT NULL DEFAULT 0,
	PRIMARY KEY (workspace_id, environment, month)
);

CREATE FUNCTION workspace_usage_follow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	-- A signature counted: the same row of the totals, by the difference.
	IF TG_OP = 'UPDATE' AND (OLD.workspace_id, OLD.environment, OLD.month) = (NEW.workspace_id, NEW.environment, NEW.month) THEN
		INSERT INTO workspace_usage (workspace_id, environment, month, count)
		VALUES (NEW.workspace_id, NEW.environment, NEW.month, NEW.count - OLD.count)
		ON CONFLICT (workspace_id, environment, month) DO UPDATE SET count = workspace_usage.count + EXCLUDED.count;
		RETURN NULL;
	END IF;
	IF TG_OP IN ('UPDATE', 'DELETE') THEN
		UPDATE workspace_usage SET count = count - OLD.count
		WHERE workspace_id = OLD.workspace_id AND environment = OLD.environment AND month = OLD.month;
	END IF;
	IF TG_OP IN ('INSERT', 'UPDATE') THEN
		INSERT INTO workspace_usage (workspace_id, environment, month, count)
		VALUES (NEW.workspace_id, NEW.environment, NEW.month, NEW.count)
		ON CONFLICT (workspace_id, environment, month) DO UPDATE SET count = workspace_usage.count + EXCLUDED.count;
	END IF;
	RETURN NULL;
END
$$;

-- No signature is counted while the totals are taken: what an older
-- instance counts during the migration waits for it, and then the trigger
-- sees it.
LOCK TABLE usage_counters IN SHARE ROW EXCLUSIVE MODE;

CREATE TRIGGER usage_counters_follow AFTER INSERT OR UPDATE OR DELETE ON usage_counters
	FOR EACH ROW EXECUTE FUNCTION workspace_usage_follow();

INSERT INTO workspace_usage (workspace_id, environment, month, count)
	SELECT workspace_id, environment, month, sum(count) FROM usage_counters GROUP BY 1, 2, 3;
