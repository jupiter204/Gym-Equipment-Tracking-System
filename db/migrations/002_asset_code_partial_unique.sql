-- Migration: 002_asset_code_partial_unique.sql
-- Description: Convert asset_code constraint to partial unique index (WHERE retired_at IS NULL)
-- Idempotent: Can be run multiple times safely.

BEGIN;

DO $$
DECLARE
    r RECORD;
BEGIN
    -- Find and drop any unique constraint on equipments(asset_code)
    FOR r IN (
        SELECT con.conname
        FROM pg_constraint con
        JOIN pg_class rel ON rel.oid = con.conrelid
        JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
        WHERE rel.relname = 'equipments'
          AND con.contype = 'u'
          AND ARRAY[con.conkey] = (
              SELECT ARRAY[ARRAY[att.attnum]]
              FROM pg_attribute att
              WHERE att.attrelid = rel.oid
                AND att.attname = 'asset_code'
          )
    ) LOOP
        EXECUTE format('ALTER TABLE equipments DROP CONSTRAINT IF EXISTS %I', r.conname);
        RAISE NOTICE 'Dropped unique constraint on equipments(asset_code): %', r.conname;
    END LOOP;
END $$;

-- Create partial unique index if it doesn't already exist
CREATE UNIQUE INDEX IF NOT EXISTS uq_equipments_asset_code_active
ON equipments (asset_code)
WHERE retired_at IS NULL;

COMMIT;
