-- +migrate Up

-- Remove default value, and change type to text
ALTER TABLE tenants
    ALTER COLUMN distribution_account_type DROP DEFAULT,
    ALTER COLUMN distribution_account_type TYPE text;

-- Drop enum
DROP TYPE distribution_account_type;

-- Create a new enum with the EXTERNAL_TSA value added
CREATE TYPE distribution_account_type AS ENUM (
    'DISTRIBUTION_ACCOUNT.STELLAR.ENV',
    'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT',
    'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT',
    'DISTRIBUTION_ACCOUNT.STELLAR.EXTERNAL_TSA'
);

-- Update column to new enum type, and set default value
ALTER TABLE tenants
    ALTER COLUMN distribution_account_type TYPE distribution_account_type USING distribution_account_type::text::distribution_account_type,
    ALTER COLUMN distribution_account_type SET DEFAULT 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT';


-- +migrate Down

-- Remove default value, and change type to text
ALTER TABLE tenants
    ALTER COLUMN distribution_account_type DROP DEFAULT,
    ALTER COLUMN distribution_account_type TYPE text;

-- Drop enum
DROP TYPE distribution_account_type;

-- Recreate enum without the EXTERNAL_TSA value
CREATE TYPE distribution_account_type AS ENUM (
    'DISTRIBUTION_ACCOUNT.STELLAR.ENV',
    'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT',
    'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT'
);

-- Update column to new enum type, and set default value
ALTER TABLE tenants
    ALTER COLUMN distribution_account_type TYPE distribution_account_type USING distribution_account_type::text::distribution_account_type,
    ALTER COLUMN distribution_account_type SET DEFAULT 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT';
