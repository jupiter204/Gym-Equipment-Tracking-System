# GETS 健身設備管理維護系統 (Gym Equipment Tracking System)

GETS 是一套專為健身房、運動中心設計的設備管理與預防性維護系統。提供設備履歷盤點、維護保養排程追蹤、營運數據分析與權限分級管理。

---

## 目錄

- [系統架構](#系統架構)
- [快速上手（開發環境）](#快速上手開發環境)
- [生產環境部署指南](#生產環境部署指南)
- [Cloudflare SSL / TLS 設定與憑證配置](#cloudflare-ssl--tls-設定與憑證配置)
- [版本升級指南 (從 Beta-1.2 升級)](#版本升級指南-從-beta-12-升級)
- [安全性功能架構](#安全性功能架構)
- [已知限制與未來展望](#已知限制與未來展望)

---

## 系統架構

- **前端 (Frontend)**: React 19 + TypeScript + Vite + Tailwind CSS + Lucide Icons
- **後端 (Backend)**: Go (Golang) RESTful API + Gin + JWT (HS256) + Swagger (swag)
- **資料庫 (Database)**: PostgreSQL 16 (含 UUIDv7 支援、交易鎖定機制)
- **反向代理 (Reverse Proxy)**: Nginx (支援 Rate Limiting、Cloudflare Real IP、SSL/TLS 終結)
- **容器化 (Containerization)**: 支援 Podman / Docker

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

### 3. 開發環境預設帳號

開發環境會自動掛載 `db/seed_dev.sql` 建立以下測試帳號：

| 帳號 (Username) | 密碼 (Password) | 角色 (Role) | 說明 |
| :--- | :--- | :--- | :--- |
| `admin` | `admin123` | `admin` (系統管理員) | 具備完整設備增刪查改、保養排程結案、人員帳號管理與統計儀表板權限 |
| `staff01` | `staff123` | `staff` (巡檢維護人員) | 具備設備檢視、巡檢回報與維護任務認領權限 |

---

## 生產環境部署指南

生產環境透過 `docker-compose.prod.yaml` 進行環境覆寫，具備以下特點：
- **環境隔離**：強制要求安全且高強度的 `JWT_SECRET` 與 `DB_PASSWORD`，若未設定或使用預設弱密碼後端將直接中斷啟動 (Fail-closed)。
- **安全覆寫**：透過 `volumes: !override` 取消掛載 `db/seed_dev.sql`，防止預設測試資料流入正式環境。
- **TLS/SSL 加密**：自動導向 HTTPS (443 埠)，阻擋未加密 HTTP 流量並注入 HSTS 安全標頭。
- **介面封閉**：在生產環境 Nginx 中將 `/swagger/*` 遮蔽為 404 Not Found。

### 1. 準備生產環境變數檔案 (`.env`)

在專案根目錄建立 `.env`（切勿將此檔案提交至 Git 倉庫）：

```bash
# 生成高強度 JWT 密鑰 (長度建議至少 32 字元，不可包含預設弱密碼字樣)
JWT_SECRET=c8f8b8941f7e3492a549d012e8b2e1f486a9a7b9319e7db410c59821d3f9b231

# PostgreSQL 資料庫密碼
DB_PASSWORD=YourStrongDatabasePassword123!

# 自訂網域名稱 (供參考或 Nginx 匹配)
DOMAIN=gets.yourdomain.com
```

> **注意**：後端內建防呆機制，若 `JWT_SECRET` 包含 `please_generate`、`change_this`、`secret_key` 等開發預設字串，系統將直接 Crash Exit。

### 2. 配置 SSL 憑證

請參閱下方 [Cloudflare SSL / TLS 設定與憑證配置](#cloudflare-ssl--tls-設定與憑證配置)，將申請的憑證放置於專案根目錄的 `ssl/` 資料夾：
- `ssl/fullchain.pem`
- `ssl/privkey.pem`

並確保私鑰檔案權限為 `600`：
```bash
chmod 600 ssl/privkey.pem
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

在乾淨無任何帳號的生產環境資料庫中，後端啟動時若偵測到 `users` 資料表為空，會自動在後端日誌中印出**單次自動生成的隨機臨時管理員密碼**：
```bash
podman compose logs backend | grep "Admin bootstrap"
```
日誌範例：
```text
[WARN] Admin bootstrap: Generated initial admin credentials: username=admin, temporary_password=<RANDOM_PASSWORD>
[WARN] Admin bootstrap: PLEASE CHANGE THIS PASSWORD IMMEDIATELY UPON FIRST LOGIN!
```
使用該臨時密碼登入後，請立即至「人員管理」頁面修改管理員密碼。

---

## Cloudflare SSL / TLS 設定與憑證配置

當您擁有自訂網域名稱並使用 Cloudflare 作為 CDN / DNS Proxy 時，請依照以下步驟設定 SSL/TLS：

### 1. Cloudflare 後台設定

1. **DNS 記錄**：
   - 將您的網域名稱（例如 `gets.yourdomain.com`）指向您的伺服器公網 IP。
   - **Proxy status** 務必切換為 **Proxied (橘色雲朵)**。
2. **SSL/TLS 加密模式**：
   - 進入 Cloudflare 控制台 -> **SSL/TLS** -> **Overview**。
   - 加密模式選擇 **Full (strict)**（嚴格加密：確保 Cloudflare 到您的 Nginx 源站之間全程採用受信任憑證加密）。
3. **申請 Cloudflare Origin CA 憑證**：
   - 進入 **SSL/TLS** -> **Origin Server** -> 點擊 **Create Certificate**。
   - 憑證有效期限可選擇長達 15 年。
   - 產生後，將視窗中的 **Origin Certificate** 內容完整複製。
   - 將視窗中的 **Private Key** 內容完整複製。

### 2. 憑證放置位置與權限設置

在專案根目錄建立 `ssl` 資料夾，並存入上述金鑰：

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

# 設置嚴格檔案存取權限 (僅擁有者可讀寫)
chmod 600 ssl/privkey.pem
chmod 644 ssl/fullchain.pem
```

### 3. Nginx Real IP 機制說明

當透過 Cloudflare 代理時，若未設定 Real IP，Nginx 記錄與 Rate Limiting 所看到的連線來源將全數為 Cloudflare 的邊緣節點 IP，導致：
- 速率限制 (Rate Limit) 誤將所有使用者視為同一來源而錯誤封鎖。
- 安全稽核日誌無法追蹤真實訪客來源。

本專案之 `nginx.prod.conf` 已預載 Cloudflare 官方 IPv4 / IPv6 網段白名單：
```nginx
set_real_ip_from 173.245.48.0/20;
set_real_ip_from 103.21.244.0/22;
...
real_ip_header CF-Connecting-IP;
```
Nginx 會自動解析 `CF-Connecting-IP` 標頭還原客戶端真實 IP，並安全套用於 `limit_req_zone` 與後端日誌中。

---

## 版本升級指南 (從 Beta-1.2 升級)

若您目前運行既有的 beta-1.2 資料庫，升級至新版時需套用結構遷移以支援設備軟刪除 (Soft-delete / `retired_at`) 與狀態完整性：

### 執行遷移腳本

在資料庫容器運行狀態下，執行遷移指令：

```bash
# 使用 Podman
podman compose exec -T db psql -U postgres -d gets < db/migrations/001_upgrade_from_beta_1_2.sql

# 使用 Docker
docker compose exec -T db psql -U postgres -d gets < db/migrations/001_upgrade_from_beta_1_2.sql
```

該腳本具備**冪等性 (Idempotent)**，多次執行安全無虞。它將：
1. 於 `equipments` 表新增 `retired_at TIMESTAMPTZ NULL` 欄位與查詢索引。
2. 將資產編號唯一性改為條件式索引 (`WHERE retired_at IS NULL`)，允許退役設備保留歷史資料且不佔用資產編號。
3. 檢查並補足 `refresh_tokens` 表與維護狀態約束。

---

## 安全性功能架構

本系統實作了業界標準之防禦與交易完整性機制：

1. **Bcrypt 72-Byte 邊界截斷防護**：
   - 由於標準 Bcrypt 演算法僅處理前 72 位元組，系統於使用者密碼輸入時進行嚴格的 UTF-8 位元組長度校驗 (`len([]byte(password)) <= 72`)，阻擋密碼被無預警截斷的安全性缺陷。
2. **原子化 Refresh Token Rotation 與並行競態保護**：
   - Refresh Token 採用單次使用即作廢機制 (Rotate-on-use)。
   - 透過資料庫交易以 `RETURNING 1` 確保原子性作廢，杜絕並行重放攻擊。
   - 針對多分頁 (Multi-tab) 同步發起 Token 刷新情境，提供 10 秒安全寬限期 (Grace Period)，避免使用者分頁遭無預警登出。
3. **Fail-Closed 撤銷驗證機制**：
   - 當 Token 撤銷狀態查詢發生資料庫連線中斷或錯誤時，驗證中介軟體一律採取拒絕存取原則 (Fail-closed)，保障系統安全。
4. **最後管理員鎖定保護**：
   - 在角色變更、帳號刪除交易中，使用 `SELECT count(*) ... FOR UPDATE` 鎖定管理員記錄，杜絕並行操作下出現「系統中無任何管理員」的死鎖窘境。
5. **密碼變更全設備強制登出**：
   - 密碼修改與現有 Refresh Token 撤銷綁定於同一個資料庫交易中，密碼一經變更即刻作廢該帳號所有已發放之 Refresh Token。
6. **設備關聯軟刪除 (Soft Delete)**：
   - 存在保養稽核記錄的設備於刪除時採用 `retired_at = NOW()` 軟刪除，維護報表歷史資料完整性；無歷史記錄的設備則直接刪除，不留多餘垃圾資料。

---

## 已知限制與未來展望

- **Token 儲存機制**：目前 Access Token 與 Refresh Token 存放於前端記憶體及 LocalStorage。未來版本可評估導入 SameSite=Strict / Secure HttpOnly Cookies 機制，進一步提升抵禦 XSS 的防禦層級。
- **雙因子認證 (2FA / TOTP)**：管理員帳號目前僅支援密碼驗證，未來規劃支援 TOTP (Time-based One-Time Password) 雙層安全認證。
- **自動化備份**：生產環境建議搭配 cron 定期執行 `pg_dump` 異地備份。
