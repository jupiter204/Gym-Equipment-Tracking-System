-- Migration: 001_upgrade_from_beta_1_2.sql
-- Description: Upgrade database schema from beta-1.2 to current release
-- Idempotency: This script can safely be run multiple times.

BEGIN;

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1. Equipments: Add retired_at for soft delete support
ALTER TABLE equipments ADD COLUMN IF NOT EXISTS retired_at TIMESTAMPTZ NULL;

-- 2. Equipments: Validate maint_interval and update CHECK constraint
DO $$
DECLARE
    c_name text;
    updated_count int;
BEGIN
    -- Fix any invalid historical maint_interval (< 1)
    UPDATE equipments SET maint_interval = 30 WHERE maint_interval < 1;
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    IF updated_count > 0 THEN
        RAISE NOTICE '已將 % 筆 maint_interval < 1 的設備保養週期調整為預設值 30 天', updated_count;
    END IF;

    -- Drop existing maint_interval check constraints
    FOR c_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'equipments'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%maint_interval%'
    LOOP
        EXECUTE format('ALTER TABLE equipments DROP CONSTRAINT %I', c_name);
    END LOOP;

    -- Add standard check constraint
    ALTER TABLE equipments ADD CONSTRAINT chk_equipments_maint_interval CHECK (maint_interval >= 1);
END $$;

-- 3. Equipments: Update status CHECK constraint (include pending_maint)
DO $$
DECLARE
    c_name text;
BEGIN
    FOR c_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'equipments'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%status%'
    LOOP
        EXECUTE format('ALTER TABLE equipments DROP CONSTRAINT %I', c_name);
    END LOOP;

    ALTER TABLE equipments ADD CONSTRAINT chk_equipments_status CHECK (status IN ('normal', 'faulty', 'pending_maint', 'repairing'));
END $$;

-- 4. Maintenance Records: Update reporter_type CHECK constraint (include system)
DO $$
DECLARE
    c_name text;
BEGIN
    FOR c_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'maintenance_records'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%reporter_type%'
    LOOP
        EXECUTE format('ALTER TABLE maintenance_records DROP CONSTRAINT %I', c_name);
    END LOOP;

    ALTER TABLE maintenance_records ADD CONSTRAINT chk_maintenance_records_reporter_type CHECK (reporter_type IN ('public', 'staff', 'system'));
END $$;

-- 5. Maintenance Records: Ensure foreign key to equipments is ON DELETE RESTRICT
DO $$
DECLARE
    fk_name text;
BEGIN
    FOR fk_name IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'maintenance_records'::regclass AND contype = 'f'
          AND confrelid = 'equipments'::regclass
    LOOP
        EXECUTE format('ALTER TABLE maintenance_records DROP CONSTRAINT %I', fk_name);
    END LOOP;

    ALTER TABLE maintenance_records
        ADD CONSTRAINT fk_maintenance_records_equipment_id
        FOREIGN KEY (equipment_id) REFERENCES equipments(lid) ON DELETE RESTRICT;
END $$;

-- 6. Refresh Tokens Table (Token Family Rotation & Revocation)
CREATE TABLE IF NOT EXISTS refresh_tokens (
    jti UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(lid) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);

-- 7. Partial Unique Index: only one active (unresolved) maintenance record per equipment
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = 'uq_one_open_record_per_equipment'
    ) THEN
        -- Check if there are duplicate unresolved records for any equipment
        IF EXISTS (
            SELECT equipment_id
            FROM maintenance_records
            WHERE is_resolved = false
            GROUP BY equipment_id
            HAVING COUNT(*) > 1
        ) THEN
            RAISE EXCEPTION '無法建立唯一索引 uq_one_open_record_per_equipment：已有設備存在多筆未解決的維修通報紀錄，請先人工處理重複紀錄後再執行此遷移！';
        ELSE
            CREATE UNIQUE INDEX uq_one_open_record_per_equipment
            ON maintenance_records (equipment_id)
            WHERE is_resolved = false;
        END IF;
    END IF;
END $$;

COMMIT;
