# GETS 健身設備管理維護系統 (Gym Equipment Tracking System)

GETS 是一套專為健身房、運動中心設計的設備管理與預防性維護系統。提供設備履歷盤點、維護保養排程追蹤、營運數據分析與權限分級管理。

---

## 目錄

- [系統架構](#系統架構)
- [快速上手（開發環境）](#快速上手開發環境)
- [生產環境部署指南](#生產環境部署指南)
- [Cloudflare SSL / TLS 設定與憑證配置](#cloudflare-ssl--tls-設定與憑證配置)
- [執行整合測試 (Integration Tests)](#執行整合測試-integration-tests)
- [版本升級指南 (從 Beta-1.2 升級)](#版本升級指南-從-beta-12-升級)
- [安全性功能架構](#安全性功能架構)
- [已知限制與未來展望](#已知限制與未來展望)

---

## 系統架構

- **前端 (Frontend)**: React 19 + TypeScript + Vite + Tailwind CSS + Lucide Icons
- **後端 (Backend)**: Go (Golang 1.25+) RESTful API + Gin + JWT (HS256) + Swagger (swag)
- **資料庫 (Database)**: PostgreSQL 15/16 (含 UUID 支援、交易鎖定機制、部分索引)
- **反向代理 (Reverse Proxy)**: Nginx (支援 Rate Limiting、Cloudflare Real IP、SSL/TLS 終結)
- **容器化 (Containerization)**: 支援 Podman (搭配 docker-compose provider) / Docker (Docker Compose ≥ 2.24.4)

---

## 快速上手（開發環境）

開發環境預設啟用測試資料、Swagger API 文件與本機 HTTP 代理。

### 1. 啟動容器服務

以 **Podman** 啟動：
```bash
podman compose up -d
```
或以 **Docker** 啟動：
```bash
docker compose up -d
```

### 2. 存取系統服務

- **前端應用入口**：[http://localhost:8000](http://localhost:8000)
- **Swagger API 文件**：[http://localhost:8000/swagger/index.html](http://localhost:8000/swagger/index.html)
- **後端 API 代理**：[http://localhost:8000/api/](http://localhost:8000/api/)
- **健康檢查**：[http://localhost:8000/healthz](http://localhost:8000/healthz)

### 3. 開發環境預設帳號

開發環境會自動掛載 `db/seed_dev.sql` 建立以下測試帳號（**僅限開發環境，正式環境不存在任何預設帳號**）：

| 帳號 (Username) | 密碼 (Password) | 角色 (Role) | 說明 |
| :--- | :--- | :--- | :--- |
| `admin` | `admin123456` | `admin` (系統管理員) | 具備完整設備增刪查改、保養排程結案、人員帳號管理與統計儀表板權限 |
| `staff01` | `admin123456` | `staff` (巡檢維護人員) | 具備設備檢視、巡檢回報與維護任務認領權限 |

### 4. 執行開發環境冒煙測試

系統提供自動化冒煙測試腳本，可用於驗證容器健康狀態與 API 運作：
```bash
bash scripts/smoke_dev.sh
```
該腳本會執行 9 項測試，確認開發帳號登入、角色存取控制、公開報修與健康檢查皆正常。

---

## 生產環境部署指南

生產環境透過 `docker-compose.prod.yaml` 進行環境覆寫，具備以下特點：
- **環境隔離**：強制要求安全且高強度的 `JWT_SECRET` 與 `DB_PASSWORD`，若未設定或使用預設弱密碼後端將直接中斷啟動 (Fail-closed)。
- **安全覆寫**：透過 `volumes: !override` 取消掛載 `db/seed_dev.sql`，防止預設測試資料流入正式環境。
- **TLS/SSL 加密**：自動導向 HTTPS (443 埠)，阻擋未加密 HTTP 流量並注入 HSTS 安全標頭。
- **介面封閉**：在生產環境 Nginx 中將 `/swagger/*` 遮蔽為 404 Not Found。

> **版本要求**：`docker-compose.prod.yaml` 使用 `!override` 語法，需使用 **Docker Compose ≥ 2.24.4**。若使用 Podman，請確保使用相容的 compose provider（例如透過 `/usr/lib/docker/cli-plugins/docker-compose`）。

### 1. 準備生產環境變數檔案 (`.env`)

在專案根目錄建立 `.env`（切勿將此檔案提交至 Git 倉庫）：

```bash
# 生成高強度 JWT 密鑰 (長度至少 32 字元，不可包含預設弱密碼字樣)
# 可執行 openssl rand -hex 32 產生
JWT_SECRET=<請執行：openssl rand -hex 32>

# PostgreSQL 資料庫密碼 (至少 8 字元，不可為 postgres 或預設弱密碼)
DB_PASSWORD=<請輸入高強度資料庫密碼>

# 自訂網域名稱 (僅供參考與記錄，Nginx 生產設定使用 server_name _)
DOMAIN=gets.yourdomain.com

# 初始管理員引導 (可選，僅在乾淨空資料庫首次啟動時設定)
BOOTSTRAP_ADMIN_USERNAME=<管理員帳號>
BOOTSTRAP_ADMIN_PASSWORD=<至少 8 字元、至多 72 位元組的強密碼>
```

> **生產環境安全護欄**：
> - `APP_ENV=production` 下，若 `JWT_SECRET` 包含 `please_generate`、`change_this`、`secret_key` 或 repo 歷史範例金鑰，系統拒絕啟動。
> - `DB_PASSWORD` 若為空、為 `postgres`、包含 `change_this` 或長度小於 8 字元，系統拒絕啟動。

### 2. 配置 SSL 憑證

請參閱下方 [Cloudflare SSL / TLS 設定與憑證配置](#cloudflare-ssl--tls-設定與憑證配置)，將申請的憑證放置於專案根目錄的 `ssl/` 資料夾：
- `ssl/fullchain.pem`
- `ssl/privkey.pem`

並確保私鑰檔案權限為 `600`：
```bash
chmod 600 ssl/privkey.pem
chmod 644 ssl/fullchain.pem
```

### 3. 一鍵啟動生產環境

使用 **Podman**：
```bash
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml --env-file .env up -d
```
或使用 **Docker**：
```bash
docker compose -f docker-compose.yaml -f docker-compose.prod.yaml --env-file .env up -d
```

### 4. 正式環境初始管理員引導 (Bootstrap Admin)

在全新無使用者的生產資料庫中：
1. 在 `.env` 中設定 `BOOTSTRAP_ADMIN_USERNAME` 與 `BOOTSTRAP_ADMIN_PASSWORD`。
2. 啟動後端容器。後端偵測到 `users` 筆數為 0 且設定了上述變數時，會自動建立初始管理員帳號，並在日誌記錄：
   ```text
   已成功建立初始管理員帳號: <帳號名稱>
   ```
3. 若 `users` 資料表為空且**未設定**上述變數，後端會輸出警告提示：
   ```text
   系統目前沒有任何使用者；請設定 BOOTSTRAP_ADMIN_USERNAME 與 BOOTSTRAP_ADMIN_PASSWORD 後重啟
   ```
4. **安全建議**：首次登入後，請立即至後台修改密碼，並從 `.env` 中移除 `BOOTSTRAP_ADMIN_USERNAME` 與 `BOOTSTRAP_ADMIN_PASSWORD`。

---

## Cloudflare SSL / TLS 設定與憑證配置

當您擁有自訂網域名稱並使用 Cloudflare 作為 CDN / DNS Proxy 時，請依照以下步驟設定 SSL/TLS：

### 1. Cloudflare 後台設定

1. **DNS 記錄**：
   - 將您的網域名稱（例如 `gets.yourdomain.com`）指向伺服器公網 IP。
   - **Proxy status** 務必切換為 **Proxied (橘色雲朵)**。
2. **SSL/TLS 加密模式**：
   - 進入 Cloudflare 控制台 -> **SSL/TLS** -> **Overview**。
   - 加密模式選擇 **Full (strict)**（確保 Cloudflare 到 Nginx 源站之間全程採用受信任憑證加密）。
3. **申請 Cloudflare Origin CA 憑證**：
   - 進入 **SSL/TLS** -> **Origin Server** -> 點擊 **Create Certificate**。
   - 憑證有效期限可選擇長達 15 年。
   - 將生成的 **Origin Certificate** 與 **Private Key** 複製。

### 2. 憑證放置位置與權限設置

在專案根目錄建立 `ssl` 資料夾並存入金鑰：

```bash
mkdir -p ssl

# 將 Origin Certificate 存為 fullchain.pem
cat << 'EOF' > ssl/fullchain.pem
-----BEGIN CERTIFICATE-----
... (貼上 Cloudflare Origin Certificate) ...
-----END CERTIFICATE-----
EOF

# 將 Private Key 存為 privkey.pem
cat << 'EOF' > ssl/privkey.pem
-----BEGIN PRIVATE KEY-----
... (貼上 Cloudflare Private Key) ...
-----END PRIVATE KEY-----
EOF

# 設置嚴格檔案存取權限 (僅擁有者可讀寫私鑰)
chmod 600 ssl/privkey.pem
chmod 644 ssl/fullchain.pem
```

### 3. Nginx Real IP 機制說明

當透過 Cloudflare 代理時，若未設定 Real IP，Nginx 記錄與限流看到的連線來源將全數為 Cloudflare 節點 IP。
本專案之 `nginx.prod.conf` 內建 Cloudflare 官方 IPv4 / IPv6 網段設定並解析 `CF-Connecting-IP` 標頭還原客戶端真實 IP。
> **備註**：Cloudflare IP 清單可能隨官方維護有所異動，建議營運維護時定期比對 [Cloudflare IP Ranges](https://www.cloudflare.com/ips/)。

---

## 執行整合測試 (Integration Tests)

系統提供專屬的隔離測試資料庫配置，確保測試執行不影響開發與生產環境資料：

```bash
# 1. 啟動隔離測試資料庫 (埠 55432, 僅載入 init.sql, 無種子資料)
podman compose -f docker-compose.test.yaml up -d db-test

# 2. 執行後端整合測試
cd backend
DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=postgres DB_PASSWORD=postgres \
DB_NAME=gets_test JWT_SECRET=integration_test_jwt_secret_32_chars_minimum_value \
REQUIRE_DB_TESTS=1 go test ./... -v -count=1

# 3. 測試完畢後銷毀測試資料庫
cd ..
podman compose -f docker-compose.test.yaml down -v
```

> **安全隔離保證**：
> - 整合測試連線池內建安全防呆，若 `DB_NAME` 不以 `_test` 結尾，測試將直接拒絕執行，防止誤連開發或生產庫。
> - 所有需要使用者權限的測試皆透過臨時帳號 (`itest_<role>_<nano>`) 動態建立，並於測試結束時透過 `t.Cleanup` 徹底清理，絕不篡改任何既有帳號。

---

## 版本升級指南 (從 Beta-1.2 升級)

若您目前運行既有的 beta-1.2 資料庫，升級至新版時需遵循以下步驟進行資料備份、資料搬遷與結構遷移：

### 1. 備份舊版資料庫

在舊環境執行 PostgreSQL 二進位備份：
```bash
# 請替換 $OLD_CONTAINER, $OLD_USER, $OLD_DB 為舊環境實際值
podman exec -t $OLD_CONTAINER pg_dump -U "$OLD_USER" -d "$OLD_DB" -Fc -f /tmp/backup_beta_1_2.dump
podman cp $OLD_CONTAINER:/tmp/backup_beta_1_2.dump ./backup_beta_1_2.dump
```

### 2. 線上 Schema 比對 (待人工比對)

若從舊版升級，建議先將線上 Schema 匯出：
```bash
podman exec -t $OLD_CONTAINER pg_dump -U "$OLD_USER" -d "$OLD_DB" -s > online_schema.sql
```
比對 `online_schema.sql` 與 `db/init.sql`，確認自訂欄位或差異處。

### 3. 資料搬遷 (Bind mount 至 Named Volume)

舊版本若使用本機目錄掛載 (`./postgres_data`)，新版 compose 已全面改用 Named Volume (`postgres_data`)。建議搬遷流程：
1. 啟動新版生產環境容器（不掛載 seed 資料）。
2. 將備份檔還原至新資料庫：
   ```bash
   podman compose exec -T db pg_restore --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB" < backup_beta_1_2.dump
   ```

### 4. 依序執行結構遷移腳本

在資料庫容器內使用環境變數執行遷移：

```bash
# 執行 001 遷移 (軟刪除欄位、保養週期校正、狀態約束)
podman compose exec -T db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < db/migrations/001_upgrade_from_beta_1_2.sql

# 執行 002 遷移 (資產編號部分唯一索引)
podman compose exec -T db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < db/migrations/002_asset_code_partial_unique.sql
```

> **遷移異動說明**：
> - `001_upgrade_from_beta_1_2.sql`：新增 `retired_at` 軟刪除欄位；若既有資料存在 `maint_interval < 1` 的非法值，將自動校正為 30 天並發出 `RAISE NOTICE` 提示；補齊 `refresh_tokens` 與狀態約束。
> - `002_asset_code_partial_unique.sql`：移除 `asset_code` 的全表唯一約束，改為條件式部分索引 (`WHERE retired_at IS NULL`)，使退役設備保留記錄同時釋出資產編號供新品重用。
> - **約束命名差異註記**：全新安裝 (`init.sql`) 之約束名稱為 PostgreSQL 自動命名（如 `equipments_status_check`）；而經由 migration 升級之資料庫為 `chk_*`。後續遷移皆透過系統目錄動態查詢定義，不依賴固定約束名稱。

### 5. 升級驗證清單

- [ ] 能以既有管理員或 bootstrap 管理員成功登入。
- [ ] 設備清單筆數、維修紀錄筆數與備份一致。
- [ ] 退役設備能正常保留於維修紀錄中。

---

## 安全性功能架構

本系統實作了業界標準之防禦與交易完整性機制：

1. **Bcrypt 72-Byte 邊界截斷防護**：
   - 由於標準 Bcrypt 僅處理前 72 位元組，系統於使用者密碼輸入時進行嚴格的 UTF-8 位元組長度校驗 (`len([]byte(password)) <= 72`)，避免過長密碼被無預警截斷。
2. **原子化 Refresh Token Rotation 與並行競態保護**：
   - Refresh Token 採用單次使用即作廢機制 (Rotate-on-use)。
   - 在單一資料庫交易內以單句 `UPDATE ... RowsAffected()` 原子性撤銷舊 Token 並寫入新 Token，若中途失敗則完整 Rollback，舊 Token 不會無故失效。
   - 針對多分頁 (Multi-tab) 同步發起 Token 刷新情境，提供 10 秒安全寬限期 (Grace Period)；超過寬限期再次重用舊 Token 則觸發 Token Reuse 警示，撤銷該使用者所有未撤銷之 Token。
3. **撤銷驗證機制 (Fail-Closed)**：
   - 在 Refresh Token 驗證流程中，若資料庫連線中斷或查無記錄，系統一律採取拒絕原則 (Fail-closed)。
4. **最後管理員鎖定保護**：
   - 在使用者角色變更或刪除交易中，使用 `SELECT lid FROM users WHERE role = 'admin' FOR UPDATE` 鎖定管理員記錄並在程式中計數，杜絕並行操作下出現「系統中無任何管理員」的狀態。
5. **密碼變更全設備強制登出**：
   - 密碼修改與現有 Refresh Token 撤銷綁定於同一個資料庫交易中，密碼一經變更即刻作廢該帳號所有已發放之 Refresh Token。
6. **設備關聯軟刪除 (Soft Delete)**：
   - 存在維修保養記錄的設備於刪除時採用 `retired_at = NOW()` 軟刪除，維護報表歷史資料完整性；無歷史記錄的設備則直接硬刪除。

---

## 已知限制與未來展望

- **Access Token 延遲生效視窗**：Access Token 為無狀態 JWT，效期為 15 分鐘。使用者變更密碼或登出後，已發出的 Access Token 在其 15 分鐘效期結束前仍可通過單純的 JWT 簽名驗證，直到需要使用 Refresh Token 時才會被伺服端徹底阻斷。
- **Token 儲存機制**：目前 Access Token 與 Refresh Token 存放於前端記憶體及 LocalStorage。未來版本可評估導入 SameSite=Strict / Secure HttpOnly Cookies 機制，進一步提升防禦層級。
- **帳號連續登入失敗鎖定**：目前未實作針對單一帳號連續密碼錯誤的臨時鎖定機制，建議搭配 Nginx 速率限制（已於生產環境配置）降低暴力破解風險。
- **雙因子認證 (2FA / TOTP)**：管理員帳號目前僅支援密碼驗證，未來規劃支援 TOTP 雙層安全認證。
- **自動化備份**：生產環境建議搭配 cron 定期執行 `pg_dump` 異地備份。
