-- Database initialization script for GETS Equipment Management System (Schema Only)

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Equipments Table
CREATE TABLE IF NOT EXISTS equipments (
    lid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    category VARCHAR(50),
    last_maint_date DATE NOT NULL DEFAULT CURRENT_DATE,
    maint_interval INT NOT NULL CHECK (maint_interval >= 1),
    status VARCHAR(20) NOT NULL DEFAULT 'normal' CHECK (status IN ('normal', 'faulty', 'pending_maint', 'repairing')),
    location VARCHAR(100),
    retired_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial Unique Index: only active (non-retired) equipments must have unique asset_code
CREATE UNIQUE INDEX IF NOT EXISTS uq_equipments_asset_code_active
ON equipments (asset_code)
WHERE retired_at IS NULL;

-- Maintenance Records Table
CREATE TABLE IF NOT EXISTS maintenance_records (
    lid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipment_id UUID NOT NULL REFERENCES equipments(lid) ON DELETE RESTRICT,
    reporter_type VARCHAR(20) NOT NULL CHECK (reporter_type IN ('public', 'staff', 'system')),
    description VARCHAR(500) NOT NULL,
    is_resolved BOOLEAN NOT NULL DEFAULT false,
    resolve_note VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial Unique Index: only one active (unresolved) maintenance record per equipment
CREATE UNIQUE INDEX IF NOT EXISTS uq_one_open_record_per_equipment
ON maintenance_records (equipment_id)
WHERE is_resolved = false;

-- Users Table
CREATE TABLE IF NOT EXISTS users (
    lid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(32) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name VARCHAR(64) NOT NULL,
    role VARCHAR(16) NOT NULL CHECK (role IN ('admin', 'staff')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Refresh Tokens Table (Token Family Rotation & Revocation)
CREATE TABLE IF NOT EXISTS refresh_tokens (
    jti UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(lid) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
