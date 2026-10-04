-- Database initialization script for GETS Equipment Management System

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Equipments Table
CREATE TABLE IF NOT EXISTS equipments (
    lid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    category VARCHAR(50),
    last_maint_date DATE NOT NULL DEFAULT CURRENT_DATE,
    maint_interval INT NOT NULL CHECK (maint_interval >= 1),
    status VARCHAR(20) NOT NULL DEFAULT 'normal' CHECK (status IN ('normal', 'faulty', 'pending_maint', 'repairing')),
    location VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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

-- Seed initial admin user: admin / admin123456
INSERT INTO users (username, password_hash, name, role)
VALUES ('admin', '$2a$10$ri5DZJg5np9NaV/Ghw49t.lnKSYV.9lJc/HcrC4YQmmFtwsUuWq0e', '系統管理員', 'admin')
ON CONFLICT (username) DO NOTHING;

-- Seed initial staff user: staff01 / admin123456
INSERT INTO users (username, password_hash, name, role)
VALUES ('staff01', '$2a$10$ri5DZJg5np9NaV/Ghw49t.lnKSYV.9lJc/HcrC4YQmmFtwsUuWq0e', '王小明 (維修專員)', 'staff')
ON CONFLICT (username) DO NOTHING;

-- Seed sample equipments
INSERT INTO equipments (asset_code, name, category, last_maint_date, maint_interval, status, location)
VALUES
    ('RUN-001', '商用電動跑步機 T-500', '有氧器材', CURRENT_DATE - INTERVAL '10 days', 30, 'normal', '1F 有氧重訓區 A01'),
    ('RUN-002', '商用電動跑步機 T-500', '有氧器材', CURRENT_DATE - INTERVAL '40 days', 30, 'normal', '1F 有氧重訓區 A02'),
    ('ELL-001', '全功能交叉橢圓機 E-300', '有氧器材', CURRENT_DATE - INTERVAL '5 days', 60, 'normal', '1F 有氧重訓區 B01'),
    ('BIK-001', '飛輪競速健身車 B-200', '有氧器材', CURRENT_DATE - INTERVAL '15 days', 30, 'normal', '2F 飛輪教室 C01'),
    ('BEN-001', '可調式奧林匹克臥推椅', '重量訓練', CURRENT_DATE - INTERVAL '20 days', 90, 'normal', '1F 重訓自由重量區 D01')
ON CONFLICT (asset_code) DO NOTHING;
