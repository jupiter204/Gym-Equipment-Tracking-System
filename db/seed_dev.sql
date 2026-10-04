-- Development Seed Data for GETS Equipment Management System
-- WARNING: Do NOT load this file in production environments!

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
ON CONFLICT (asset_code) WHERE retired_at IS NULL DO NOTHING;
