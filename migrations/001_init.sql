-- businesses: one row per webhook consumer.
-- For the MVP there is exactly one row: business_id = 'linq_mvp'.
CREATE TABLE IF NOT EXISTS businesses (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    business_id      VARCHAR(64)   NOT NULL,
    name             VARCHAR(255)  NOT NULL,
    webhook_url      VARCHAR(2048) NOT NULL,
    webhook_secret   VARCHAR(512)  NOT NULL,
    derivation_index INTEGER       NOT NULL DEFAULT 0,
    active           BOOLEAN       NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_business_id
    ON businesses (business_id)
    WHERE deleted_at IS NULL;

-- wallet_addresses: the indexer reads this for the bloom filter.
-- The dispatcher reads it to route events → businesses.
-- This backend writes to it at /wallet/register.
-- Column types use plain VARCHAR (no Postgres enum) to match the b2b indexer SQL.
CREATE TABLE IF NOT EXISTS wallet_addresses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    address     VARCHAR(255) NOT NULL,
    type        VARCHAR(64)  NOT NULL,
    standard    VARCHAR(64),
    business_id VARCHAR(64)  NOT NULL,
    asset_type  VARCHAR(255),
    active      BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_address_network
    ON wallet_addresses (address, type)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_wallet_business_id  ON wallet_addresses (business_id);
CREATE INDEX IF NOT EXISTS idx_wallet_address_type ON wallet_addresses (type);
CREATE INDEX IF NOT EXISTS idx_wallet_created_at   ON wallet_addresses (created_at);
