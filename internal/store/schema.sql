CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS appointments (
    id         UUID        PRIMARY KEY DEFAULT uuidv7(),
    trainer_id UUID        NOT NULL,
    user_id    UUID        NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT appointments_valid_range CHECK (starts_at < ends_at),
    -- Also serves as the index for trainer + time range lookups.
    CONSTRAINT appointments_no_overlap EXCLUDE USING gist (
        trainer_id WITH =,
        tstzrange(starts_at, ends_at) WITH &&
    )
);
