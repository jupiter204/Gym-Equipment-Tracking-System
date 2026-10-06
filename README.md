# GETS — Gym Equipment Tracking System
> 健身設備管理維護系統

GETS 是一套專為**健身房、運動中心**設計的全端設備管理系統，提供設備履歷盤點、預防性維護排程、多角色權限管控與營運數據儀表板。

---

## 目錄

- [系統架構](#系統架構)
- [功能概覽](#功能概覽)
- [快速上手（開發環境）](#快速上手開發環境)
- [生產環境部署指南](#生產環境部署指南)
- [Cloudflare SSL / TLS 設定](#cloudflare-ssl--tls-設定)
- [執行整合測試](#執行整合測試)
- [PostgreSQL 15 → 18 升級指南](#postgresql-15--18-升級指南)
- [版本升級指南（從 Beta-1.2 升級）](#版本升級指南從-beta-12-升級)
- [安全性功能架構](#安全性功能架構)
- [已知限制與未來展望](#已知限制與未來展望)

---

## 系統架構

```
┌─────────────────────────────────────────────────────────┐
│                     Client (Browser / PWA)               │
└───────────────────────┬─────────────────────────────────┘
                        │ HTTPS
┌───────────────────────▼─────────────────────────────────┐
│               Nginx (Reverse Proxy)                      │
│   Rate Limiting · Cloudflare Real IP · SSL/TLS 終結      │
└──────┬────────────────────────────────┬─────────────────┘
       │ /                              │ /api/
┌──────▼──────┐                 ┌───────▼────────┐
│  Frontend   │                 │    Backend     │
│  React 19   │                 │  Go / Gin      │
│  TypeScript │                 │  REST API      │
│  Vite · PWA │                 │  JWT (HS256)   │
└─────────────┘                 └───────┬────────┘
                                        │
                                ┌───────▼────────┐
                                │   PostgreSQL   │
                                │       18       │
                                └────────────────┘
```

### 技術棧

| 層級 | 技術 |
|------|------|
| **前端** | React 19 · TypeScript · Vite 8 · Tailwind CSS 4 · Recharts · Lucide Icons · PWA |
| **後端** | Go 1.27+ · Gin · JWT (HS256) · pgx/v5 · Swagger (swag) · cron/v3 |
| **資料庫** | PostgreSQL 18（映像 postgres:18.6-alpine3.24 · UUID · 交易鎖定 · 部分索引） |
| **反向代理** | Nginx（Rate Limiting · Cloudflare Real IP · TLS 終結） |
| **容器化** | Docker Compose ≥ 2.24.4 / Podman Compose ≥ 1.4.0 |

> **已驗證版本**：Go 1.27.1、PostgreSQL 18.6

---

## 功能概覽

| 功能模組 | 管理員 (admin) | 巡檢人員 (staff) |
|----------|:--------------:|:----------------:|
| 設備清單 CRUD | ✅ | 👁 唯讀 |
| 設備掃碼公開報修 | ✅ | ✅ |
| 維護任務排程與結案 | ✅ | ✅ 可認領 |
| 帳號管理 | ✅ | ❌ |
| 統計儀表板 & 分析圖表 | ✅ | ❌ |
| Swagger API 文件 | 開發環境 | 開發環境 |

### 前端頁面結構

```
src/pages/
├── auth/
│   └── Login.tsx               # 登入頁
├── admin/
│   ├── Dashboard.tsx           # 主儀表板
│   ├── EquipmentList.tsx       # 設備管理
│   ├── MaintenanceTasks.tsx    # 維護任務
│   ├── Analytics.tsx           # 數據分析
│   └── UserManagement.tsx      # 帳號管理
└── public/
    └── ReportEquipment.tsx     # 設備公開報修（無需登入）
```

---

## 快速上手（開發環境）

開發環境預設啟用測試種子資料、Swagger API 文件與本機 HTTP 代理。

### 前置需求

- Docker Compose ≥ 2.24.4 **或** Podman Compose ≥ 1.4.0

### 1. 啟動容器服務

```bash
# Podman
podman compose up -d

# 或 Docker
docker compose up -d
```

### 2. 存取服務

| 服務 | 網址 |
|------|------|
| 前端應用 | http://localhost:8000 |
| Swagger API 文件 | http://localhost:8000/swagger/index.html |
| 後端 API 代理 | http://localhost:8000/api/ |
| 健康檢查 | http://localhost:8000/healthz |

### 3. 預設測試帳號

> **注意**：以下帳號僅存在於開發環境（由 `db/seed_dev.sql` 自動建立），**正式環境不存在任何預設帳號**。

| 帳號 | 密碼 | 角色 | 權限說明 |
|------|------|------|----------|
| `admin` | `admin123456` | `admin` | 完整設備增刪查改、保養結案、帳號管理、統計儀表板 |
| `staff01` | `admin123456` | `staff` | 設備檢視、巡檢回報、任務認領 |

### 4. 冒煙測試

```bash
bash scripts/smoke_dev.sh
```

執行 9 項自動化測試，驗證開發帳號登入、角色存取控制、公開報修與健康檢查。

---

## 生產環境部署指南

生產環境透過 `docker-compose.prod.yaml` 覆寫設定，具備以下安全特性：

- **Fail-Closed 啟動護欄**：弱密碼或未設定 `JWT_SECRET` / `DB_PASSWORD` 時後端直接拒絕啟動
- **種子資料隔離**：`!override` 語法取消掛載 `db/seed_dev.sql`，防止測試資料進入正式庫
- **強制 TLS**：HTTP → HTTPS 重導向，注入 HSTS 安全標頭
- **封閉 Swagger**：生產環境 `/swagger/*` 一律回應 404

> **版本要求**：`!override` 語法需要 **Docker Compose ≥ 2.24.4** 或 **podman-compose ≥ 1.4.0**。
> 若出現 `could not determine a constructor for the tag '!override'`，請升級 Compose provider：
> ```bash
> pip install --user -U podman-compose
> ```

### 1. 建立 `.env` 環境變數檔

在專案根目錄建立 `.env`（**切勿提交至 Git**）：

```bash
# 生成高強度 JWT 密鑰（至少 32 字元）
JWT_SECRET=$(openssl rand -hex 32)

# PostgreSQL 資料庫密碼（至少 8 字元，不可為 postgres）
DB_PASSWORD=<請輸入高強度密碼>

# 自訂網域（供記錄用）
DOMAIN=gets.yourdomain.com

# 初始管理員引導（乾淨空資料庫首次啟動時使用，之後請移除）
BOOTSTRAP_ADMIN_USERNAME=<管理員帳號>
BOOTSTRAP_ADMIN_PASSWORD=<至少 8 字元的強密碼>
```

**啟動護欄規則**：
- `JWT_SECRET` 含有 `please_generate`、`change_this` 等字樣 → 拒絕啟動
- `DB_PASSWORD` 為空、`postgres`、長度 < 8 或含 `change_this` → 拒絕啟動

### 2. 配置 SSL 憑證

請參考 [Cloudflare SSL / TLS 設定](#cloudflare-ssl--tls-設定) 取得憑證後，放置至：

```
ssl/
├── fullchain.pem   (Origin Certificate)
└── privkey.pem     (Private Key, 權限應為 600)
```

```bash
chmod 600 ssl/privkey.pem
chmod 644 ssl/fullchain.pem
```

### 3. 啟動生產環境

```bash
# Podman
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml --env-file .env up -d

# 或 Docker
docker compose -f docker-compose.yaml -f docker-compose.prod.yaml --env-file .env up -d
```

### 4. Rootless Podman 特權連接埠處理

Rootless Podman 預設無法監聽 80 / 443 特權埠。請擇一解決：

**方法 A（推薦）：調整核心參數（重開機仍生效）**

```bash
echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/99-unprivileged-ports.conf
sudo sysctl --system
```

**方法 B：改用 Rootful Podman**

```bash
sudo podman compose -f docker-compose.yaml -f docker-compose.prod.yaml --env-file .env up -d
```

### 5. Bootstrap 管理員引導

全新空資料庫首次啟動流程：

1. 在 `.env` 設定 `BOOTSTRAP_ADMIN_USERNAME` 與 `BOOTSTRAP_ADMIN_PASSWORD`
2. 啟動後端，日誌將顯示：
   ```
   已成功建立初始管理員帳號: <帳號名稱>
   ```
3. 登入後立即修改密碼，並從 `.env` 移除上述兩個變數

> 若資料表為空且未設定上述變數，後端會輸出警告提示但不阻斷啟動。

---

## Cloudflare SSL / TLS 設定

### 1. Cloudflare 後台設定

1. **DNS 記錄**：將網域指向伺服器 IP，Proxy status 設為 **Proxied（橘色雲朵）**
2. **SSL/TLS 加密模式**：選擇 **Full (strict)**（確保全程加密）
3. **申請 Origin CA 憑證**：SSL/TLS → Origin Server → Create Certificate（有效期最長 15 年）

### 2. 放置憑證

```bash
mkdir -p ssl

# 貼入 Origin Certificate
cat << 'EOF' > ssl/fullchain.pem
-----BEGIN CERTIFICATE-----
... (Cloudflare Origin Certificate) ...
-----END CERTIFICATE-----
EOF

# 貼入 Private Key
cat << 'EOF' > ssl/privkey.pem
-----BEGIN PRIVATE KEY-----
... (Cloudflare Private Key) ...
-----END PRIVATE KEY-----
EOF

chmod 600 ssl/privkey.pem
chmod 644 ssl/fullchain.pem
```

### 3. Nginx Real IP 機制

透過 Cloudflare 代理時，若未設定 Real IP，Nginx 看到的來源 IP 全為 Cloudflare 節點 IP，導致速率限制失效。

本專案 `nginx.prod.conf` 已內建 Cloudflare 官方 IPv4 / IPv6 網段，並解析 `CF-Connecting-IP` 標頭還原真實客戶端 IP。

> 建議定期比對 [Cloudflare IP Ranges](https://www.cloudflare.com/ips/) 確認清單仍為最新。

---

## 執行整合測試

系統提供獨立隔離的測試資料庫，確保測試不影響開發與生產環境：

```bash
# 1. 啟動隔離測試資料庫（埠 55432，無種子資料）
podman compose -f docker-compose.test.yaml up -d db-test

# 2. 執行整合測試
cd backend
DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=postgres DB_PASSWORD=postgres \
DB_NAME=gets_test JWT_SECRET=integration_test_jwt_secret_32_chars_minimum_value \
REQUIRE_DB_TESTS=1 go test ./... -v -count=1

# 3. 銷毀測試資料庫
cd ..
podman compose -f docker-compose.test.yaml down -v
```

**安全隔離保證**：
- `DB_NAME` 不以 `_test` 結尾時，測試直接拒絕執行，防止誤連開發或生產庫
- 所有需要使用者權限的測試皆建立臨時帳號（`itest_<role>_<nano>`），並於測試結束時透過 `t.Cleanup` 自動清理

---

## PostgreSQL 15 → 18 升級指南

本專案自此版本起資料庫由 PostgreSQL 15 升級至 **PostgreSQL 18.6**，資料卷由 `postgres_data` 改為 `postgres18_data`，掛載路徑為 `/var/lib/postgresql`。

依據部署環境與現有資料庫版本，請選擇對應的升級方式：

### 情境 A：開發環境（無須保留舊資料）

若本機開發環境無需保留既有資料，可直接清除舊卷並重新啟動：

```bash
# 1. 停止舊 stack 並清除舊資料卷
podman compose down -v

# 2. 啟動新 stack（自動初始化 PostgreSQL 18 與測試種子資料）
podman compose up -d
```

> **注意**：舊具名卷 `postgres_data` 會留在系統中成為孤兒卷（Orphan Volume），若確定不再需要可手動清理：
> ```bash
> podman volume rm database_final_postgres_data
> ```

---

### 情境 B：正式環境（目前版本為 PostgreSQL 15）

適用於已處於 PostgreSQL 15 正式版本、已有營運資料（資料庫結構已包含 `refresh_tokens`）之環境。

> **回滾保險**：操作過程中**切勿加上 `-v`**。舊具名卷 `postgres_data` 與備份檔案將完整保留，若升級過程遇異常可隨時切回舊版程式碼直接重啟舊 stack。

```bash
# 1. 在「舊 stack（仍是 PostgreSQL 15）」上備份
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml exec -T db sh -c \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > backup_pg15.dump

# 2. 停止舊 stack（保留舊資料卷作為回滾保險）
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml down

# 3. 切換至新版程式碼後，僅啟動新的 PostgreSQL 18 資料庫並等待健康檢查通過
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml up -d db

# 4. 還原備份資料至 PostgreSQL 18
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml exec -T db sh -c \
  'pg_restore --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < backup_pg15.dump

# 5. 啟動其餘服務並驗證系統
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml up -d
```

- **無須額外步驟**：此備份已包含完整的資料結構與 `refresh_tokens`，因此**不需要**「先 DROP `refresh_tokens`」步驟，亦**不需要**執行 `001` 或 `002` 遷移腳本。
- **回滾方式**：在驗證完全通過前請妥善保留 `backup_pg15.dump` 與舊卷 `postgres_data`；若需回滾，只需切換回舊版 Git commit 並使用舊版 compose 啟動舊卷即可。

---

### 情境 C：正式環境（仍停留在 Beta-1.2）

若正式環境仍停留在更早期的 Beta-1.2 版本（尚未套用 `001` 與 `002` 遷移腳本），請直接參考下方的 [從 Beta-1.2 升級](#版本升級指南從-beta-12-升級) 流程進行升級，目標資料庫為新版的 PostgreSQL 18。

---

## 版本升級指南（從 Beta-1.2 升級）

> **注意**：新版 Compose 資料卷已升級為 `postgres18_data`、容器內掛載點為 `/var/lib/postgresql`。以下流程將 Beta-1.2 備份資料匯入新版 PostgreSQL 18，並補齊 Schema 遷移腳本。

### 1. 備份舊版資料庫

```bash
podman exec -t $OLD_CONTAINER pg_dump -U "$OLD_USER" -d "$OLD_DB" -Fc -f /tmp/backup_beta_1_2.dump
podman cp $OLD_CONTAINER:/tmp/backup_beta_1_2.dump ./backup_beta_1_2.dump
```

### 2. Schema 比對（建議）

```bash
podman exec -t $OLD_CONTAINER pg_dump -U "$OLD_USER" -d "$OLD_DB" -s > online_schema.sql
# 比對 online_schema.sql 與 db/init.sql，確認自訂欄位差異
```

### 3. 資料搬遷

舊版若使用本機目錄掛載（`./postgres_data`）或舊版具名卷（`postgres_data`），新版已改用 Named Volume（`postgres18_data`，掛載於 `/var/lib/postgresql`）：

```bash
# 3-1 移除外鍵約束阻礙（避免 pg_restore 時 DROP TABLE users 失敗）
podman compose exec -T db sh -c \
  'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "DROP TABLE IF EXISTS refresh_tokens"'

# 3-2 還原備份資料
podman compose exec -T db sh -c \
  'pg_restore --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < backup_beta_1_2.dump
```

### 4. 執行 Schema 遷移

```bash
# 001：軟刪除欄位、保養週期校正、狀態約束
podman compose exec -T db sh -c \
  'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < db/migrations/001_upgrade_from_beta_1_2.sql

# 002：資產編號部分唯一索引
podman compose exec -T db sh -c \
  'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < db/migrations/002_asset_code_partial_unique.sql
```

**遷移異動說明**：

| Migration | 說明 |
|-----------|------|
| `001_upgrade_from_beta_1_2.sql` | 新增 `retired_at` 軟刪除欄位；自動校正 `maint_interval < 1` 的非法值為 30 天；補齊 `refresh_tokens` 與狀態約束 |
| `002_asset_code_partial_unique.sql` | 移除 `asset_code` 全表唯一約束，改為條件式部分索引（`WHERE retired_at IS NULL`），退役設備保留記錄同時釋出資產編號 |

> **約束命名差異**：全新安裝（`init.sql`）的約束由 PostgreSQL 自動命名（如 `equipments_status_check`）；經由 migration 升級的資料庫為 `chk_*`。後續遷移透過系統目錄動態查詢定義，不依賴固定約束名稱。

### 5. 升級驗證清單

- [ ] 能以既有管理員或 bootstrap 管理員帳號成功登入
- [ ] 設備清單筆數、維修記錄筆數與備份一致
- [ ] 退役設備能正常保留於維修記錄中

---

## 安全性功能架構

| 機制 | 說明 |
|------|------|
| **Bcrypt 72-Byte 邊界防護** | 密碼輸入時嚴格校驗 UTF-8 位元組長度（`len([]byte(password)) <= 72`），避免超長密碼被無預警截斷 |
| **原子化 Refresh Token Rotation** | Rotate-on-use；單一交易內原子撤銷舊 Token 並寫入新 Token，中途失敗完整 Rollback |
| **Multi-Tab 並行競態保護** | 10 秒安全寬限期允許多分頁同步刷新；超過寬限期再次重用舊 Token 則觸發全帳號 Token 撤銷 |
| **Fail-Closed 撤銷驗證** | Refresh Token 驗證流程中，資料庫連線中斷或查無記錄一律採拒絕原則 |
| **最後管理員鎖定保護** | 角色變更或刪除交易中使用 `SELECT ... FOR UPDATE` 防止並行操作導致系統無管理員 |
| **密碼變更全設備登出** | 密碼修改與 Refresh Token 撤銷綁定於同一交易，密碼一改即刻作廢所有已發放 Token |
| **設備關聯軟刪除** | 有維修記錄的設備以 `retired_at = NOW()` 軟刪除，保護報表歷史資料完整性 |

---

## 已知限制與未來展望

| 項目 | 說明 |
|------|------|
| **Access Token 延遲生效視窗** | JWT Access Token 效期 15 分鐘，密碼變更或登出後已發出的 Token 在效期內仍可通過簽名驗證，直到 Refresh 時才被阻斷 |
| **Token 儲存機制** | 目前存放於前端記憶體與 LocalStorage；未來可評估導入 SameSite=Strict / Secure HttpOnly Cookie |
| **帳號暴力破解防護** | 未實作單帳號連續錯誤鎖定；建議搭配已配置的 Nginx 速率限制降低風險 |
| **雙因子認證 (2FA / TOTP)** | 規劃中，管理員帳號目前僅支援密碼驗證 |
| **自動化備份** | 建議生產環境搭配 cron 定期執行 `pg_dump` 異地備份 |
