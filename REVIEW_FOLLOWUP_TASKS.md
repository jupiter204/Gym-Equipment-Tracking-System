# 修改任務單：`database_final` 審查後續

> 對象：負責修改原始碼的 agent
> 審查基準：commit `58a852b`（`fix(nginx): relax login rate limiting...`），行號以該版本為準，修改後可能位移。
> 對照文件：`CODE_IMPROVEMENTS.md`（上一輪的改進清單）。本文件只列**上一輪之後仍需處理的項目**。

---

## 0. 先讀這裡

### 0.1 已確認的決策（不要推翻）

| 決策 | 內容 |
|---|---|
| 預設測試帳號 | **保留** `admin` / `staff01` 與範例設備，方便測試。 |
| JWT_SECRET | 開發時可用預設佔位值；**正式上線時會更換**。 |
| 護欄 | 為了讓「上線時更換」不依賴記憶，要加上 production 模式的強制檢查（見 T2）。開發流程必須維持「零設定 `docker compose up` 即可跑」。 |

### 0.2 審查驗證了什麼、沒驗證什麼

- **已實際執行（前端）**：`npm ci`、`tsc -b`（通過）、`vite build`（通過）、`eslint .`（4 個錯誤，皆為規則風格問題，見 T15）。
- **未能執行（後端）**：審查環境沒有 Go，無法編譯、無法跑測試。後端結論來自逐行閱讀。**你修改後務必自己跑 `go build ./... && go vet ./... && go test ./...`。**
- **未執行**：`docker compose build/up`、真實資料庫。`backend/Dockerfile` 使用 `golang:1.26.2-alpine3.22` 與 `alpine:3.22.4`，審查者無法確認這些 tag 存在，請先 `docker compose build` 驗證。
- 標示「依推演」的項目是從程式碼流程推論，未實測；修好後請寫測試或手動重現確認。

### 0.3 已正確、請勿改壞（回歸檢查清單）

JWT 啟動檢查與 `InitJWT` 呼叫順序、access/refresh `type` claim 檢查、登入 dummy bcrypt 防 timing、公開報修（固定 `reporter_type='public'`、8 KB body 上限、partial unique index + `23505`→409）、resolve 冪等（409/404）、設備與使用者輸入驗證與 409、防自刪／自降級、cron 使用 `Asia/Taipei` + tzdata、`http.Server` timeout + graceful shutdown + `/healthz`、backend Dockerfile（非 root、依賴快取、HEALTHCHECK）、compose（db healthcheck、named volume、`backend-net internal: true`、init.sql 掛載）、`url.URL` 組 DB 連線字串、前端 `RequireAuth`、已移除 `any` 與 `console.log`、PWA 圖示與 `devOptions.enabled:false`、nginx 登入限流與 XFF 覆寫（`$remote_addr`）、CSP 標頭。

### 0.4 執行順序建議

`T4`（小、獨立）→ `T2` → `T1` → `T5` → `T3` → 其餘 P1 → P2。每完成一項就跑一次 §6 的驗證。

---

## 1. P0：部署前必修

### T1. 還原正式環境的 HTTPS

- **問題**：修改前（commit `f8e9ac6`）`nginx.conf` 有 `listen 443 ssl`、Let's Encrypt 憑證路徑、80→443 轉址，compose 開 `443:443` 並掛 `./ssl`。現在 `nginx.conf` 只有 `listen 80`，compose 沒有 443。正式環境登入帳密與 JWT 會以明文傳輸。上一輪清單的原意是「**另外提供**不需憑證的 dev 設定」，不是取代正式設定。
- **參考**：`git show f8e9ac6:nginx.conf`、`git show f8e9ac6:docker-compose.yaml`。
- **做法**：
  1. 拆成兩份 nginx 設定：
     - `nginx.dev.conf`：僅 80，無憑證（即目前內容）。
     - `nginx.prod.conf`：80 → 301 到 443；443 `ssl` + `TLSv1.2/1.3`；憑證路徑與網域**不要寫死個人網域**，可用 nginx 官方映像的 `/etc/nginx/templates/*.template` + `envsubst`（例如 `${SERVER_NAME}`）。
  2. 兩份都保留：`login_limit` / `public_api_limit` zone、安全標頭、`X-Forwarded-For $remote_addr`。prod 額外加 `Strict-Transport-Security`。
  3. compose 拆分：**`docker-compose.yaml` 預設為 dev**（掛 `nginx.dev.conf`，不需憑證）；新增 `docker-compose.prod.yaml` override（掛 `nginx.prod.conf`、開 `443:443`、掛 `./ssl`）。啟動方式寫進 README：
     - dev：`docker compose up -d`
     - prod：`docker compose -f docker-compose.yaml -f docker-compose.prod.yaml up -d`
  4. 若正式環境是由其他反向代理（Cloudflare 等）終結 TLS，需在 README 註明，並在 nginx 設定 `real_ip_header` / `set_real_ip_from`，否則限流只會看到代理 IP。
- **驗收**：dev 不放任何憑證也能在 `http://localhost:8000` 使用；prod override 下 80 會 301 到 443，443 提供 TLS；`nginx -t` 通過。

### T2. 密鑰與預設帳號：保留測試便利，加上正式環境護欄

- **背景**：`docker-compose.yaml:22` 的 `JWT_SECRET` fallback 與 `.env`/`.env.example` 的值是公開字串（73 字元），能通過 `InitJWT` 的 ≥32 檢查；`init.sql:56-64` 種子帳號密碼為 `admin123456`。這在開發可接受，**但正式環境若忘記更換就等於公開 admin 權限**。
- **做法**：
  1. **新增 `APP_ENV`**：base compose 設 `APP_ENV=${APP_ENV:-development}`；`docker-compose.prod.yaml` 設 `APP_ENV=production`。
  2. **`middleware.InitJWT()`**：維持 ≥32 字元；當 `APP_ENV=production` 時，另外拒絕已知佔位值（完全等於 `.env.example` 的值，或包含 `please_generate` / `change_this`），錯誤訊息要明確說「請更換 JWT_SECRET」。補單元測試（development 接受佔位值、production 拒絕）。
  3. **prod override** 對必要變數使用「缺少就拒絕啟動」語法：
     ```yaml
     - JWT_SECRET=${JWT_SECRET:?請在 .env 設定 JWT_SECRET}
     - DB_PASSWORD=${DB_PASSWORD:?請在 .env 設定 DB_PASSWORD}
     ```
     db 服務同樣處理 `POSTGRES_PASSWORD`。base（dev）的 fallback 維持不動。
  4. **種子資料與 schema 分離**：
     - `db/init.sql` 只保留 schema（extension、tables、index、constraints）。
     - 新增 `db/seed_dev.sql`，放 `admin` / `staff01` 與範例設備。
     - 掛載（Postgres 依檔名排序執行）：
       - base（dev）：`./db/init.sql:/docker-entrypoint-initdb.d/01-init.sql:ro` 與 `./db/seed_dev.sql:/docker-entrypoint-initdb.d/02-seed_dev.sql:ro`
       - prod override：只保留 01，**不掛 seed**。需要 `!override`/`!reset` 或將 seed 掛載只寫在 dev 專用 override 檔，請選擇實際可行的 compose 寫法並驗證。
  3. **（選做）正式環境的初始 admin**：啟動時若 `users` 為空且設定了 `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD`，建立一位 admin（密碼走既有 bcrypt 與長度驗證）。若不做，README 需寫明手動建立方式。
  4. README 新增 **Production checklist**：更換 `JWT_SECRET`、`DB_PASSWORD`；確認未載入 seed；移除或更改所有預設帳號；`.env` 不隨 zip/版控外流。
- **驗收**：
  - dev：不建立 `.env`、不設任何變數，`docker compose up` 可啟動，且能用 `admin / admin123456` 登入。
  - prod：未設定 `JWT_SECRET` 時 `docker compose ... config` 或啟動即失敗；`JWT_SECRET` 設成佔位值時後端啟動失敗；prod 資料庫內**沒有** `admin123456` 的使用者與範例設備。

### T3. 分頁只做了後端，前端與統計沒有配合

- **問題**：`common.go: parsePagination` 讓三個列表 API 預設只回 **50 筆（上限 100）**，但：
  - `Dashboard.tsx:15`、`Analytics.tsx:33-34`、`EquipmentList.tsx:44`、`MaintenanceTasks.tsx:17`、`UserManagement.tsx:39` 都沒帶 `limit/offset`，也沒有換頁 UI。
  - Dashboard「總器材數」與狀態圓餅圖、Analytics 的分類統計／近六個月趨勢／CSV 匯出，都是用回傳陣列直接計算。
  - 後果：設備或紀錄超過 50 筆後數字**默默變少，不會有任何錯誤**。Analytics 抓全部歷史紀錄且 cron 會持續新增，最先出錯。`MaintenanceTasks`（`resolved=false`）同樣會漏掉第 51 筆之後的待辦。
  - 目前種子資料只有 5 台，所以現在看不出來。
- **做法**：
  1. **後端**：
     - 三個列表 API 回應加 `X-Total-Count` header（維持 body 為陣列，避免破壞既有格式）。`GetMaintenanceRecords` 的總數須套用相同的 `resolved` 篩選。
     - 排序加穩定的第二鍵：`ORDER BY created_at DESC, lid`（避免相同 `created_at` 時換頁重複或遺漏）。
     - 新增 `GET /api/private/stats`（`admin`、`staff` 皆可），在資料庫端聚合，至少包含：設備總數與各 `status` 數量；各設備分類的維修紀錄數（分類為空以「未分類」計）；近六個月每月的「通報中（未解決）／已完成（已解決）」數量（以 `Asia/Taipei` 時區、依 `created_at`）。**先讀 `Analytics.tsx` 與 `Dashboard.tsx` 現有計算邏輯，確保聚合結果與原行為一致。**
  2. **前端**：
     - `EquipmentList`、`MaintenanceTasks`、`UserManagement` 加分頁（讀 `X-Total-Count`）。
     - `Dashboard`、`Analytics` 改用 `/private/stats`，不再用陣列長度計算。
     - Analytics 的 CSV 匯出需要全量資料：用 `limit=100` 迴圈取完，或新增專用匯出端點。
  3. 更新 `types.ts` 與 swagger（見 T12）。
- **驗收**：用腳本灌入 120 台設備、300 筆維修紀錄後，Dashboard 總數 = 120、各狀態數量正確；Analytics 圖表與 CSV 筆數 = 300；設備列表可翻到最後一頁；維修任務頁能看到全部未解決紀錄。

### T4. 登入頁在連續輸錯後會卡死（依推演，未實測）

- **位置**：`frontend/src/services/apiClient.ts:65-72`
- **問題**：`Login.tsx` 以 `apiClient.post('/auth/login')` 登入，401 會進入回應攔截器。攔截器在 `isRefreshing = true`（第 66 行）之後，若沒有 refresh token（第 68-72 行）就直接 `return`，**沒有進入 `finally`，`isRefreshing` 永遠是 `true`**。第二次輸入錯誤密碼時，401 會被丟進永遠不會 resolve 的 `failedQueue`，`await` 永不返回，`setIsLoading(false)` 不會執行，按鈕一直轉圈、輸入框被停用，只能重新整理頁面。另外，若 refresh 回應沒有 `access_token`（沒拋錯）也會讓排隊中的請求永遠懸空。
- **做法**：
  1. 攔截器開頭先排除認證端點，不要讓它們走刷新流程：
     ```ts
     const url = originalRequest.url ?? '';
     if (url.includes('/auth/login') || url.includes('/auth/refresh')) {
       return Promise.reject(error);
     }
     ```
  2. 把 `isRefreshing = true` 之後的**所有路徑**包進同一個 `try/finally`，`finally` 一律重置 `isRefreshing=false`；「沒有 refresh token」「回應缺 `access_token`」都要 `processQueue(error)` 並 reject，不可留下未結算的 Promise。
  3. **多分頁處理**（搭配 T6）：送出 refresh 前先記下當時的 `refresh_token`；若 refresh 失敗（401）時發現 `localStorage` 的 `refresh_token` 已與送出的不同，代表別的分頁已刷新成功，應改用目前的 `access_token` 重試原請求，**不要登出**。
- **驗收**（建議用 vitest + axios mock，或至少手動）：連續輸入 3 次錯誤密碼，每次都顯示錯誤訊息且按鈕恢復可按；同時發出 5 個 401 請求只會送出 1 次 refresh；refresh 失敗時所有排隊請求都被 reject 且導向 `/login?expired=true`。

### T5. 既有資料庫不會自動升級（migration）

- **問題**：
  - `db/init.sql` 只在資料目錄為空時執行（`docker-entrypoint-initdb.d` 的行為）。
  - compose 同時把 `./postgres_data`（bind mount）換成 named volume `postgres_data`，**舊資料不會被帶到新卷**，新環境是全新空庫。
  - `db/init.sql` 是這次提交才第一次加入，撰寫時沒有看到線上資料庫，欄位可能與實際不符。
  - 舊 schema 缺少 `refresh_tokens`、partial unique index，`reporter_type` 的 CHECK 也可能不含 `system`、`status` 可能不含 `pending_maint`。缺表時後端仍會「看起來能跑」（`auth.go:75` 的 INSERT 錯誤被吞掉，見 T6），但 token 撤銷會默默失效。
- **做法**：
  1. 新增 `db/migrations/001_upgrade_from_beta_1_2.sql`，**必須冪等**（可重複執行）：
     - `CREATE TABLE IF NOT EXISTS refresh_tokens (...)` 與其 index。
     - 建立 `uq_one_open_record_per_equipment` 之前，先檢查是否已有同設備多筆未解決紀錄（有則列出並要求先人工處理，因為建立 unique index 會失敗）。
     - 以 `ALTER TABLE ... DROP CONSTRAINT IF EXISTS ..., ADD CONSTRAINT ...` 更新 `reporter_type`、`status`、`maint_interval >= 1` 的 CHECK。**先用 `\d table` 確認實際 constraint 名稱**；`maint_interval` 若有 <1 的既有資料需先修正。
     - 確認 `maintenance_records.equipment_id` 的外鍵為 `ON DELETE RESTRICT`（不是 CASCADE）。
  2. README 新增「從 beta-1.2 升級」：
     - 先 `pg_dump` 備份。
     - 資料搬遷二選一並寫清楚指令：(a) 保留舊 bind mount 路徑；(b) 從舊容器 `pg_dump` 再 restore 到新 named volume。
     - 建議用 `pg_dump -s` 倒出線上 schema，與 `init.sql` 比對，差異以線上為準修正 `init.sql`。**此步驟需要人工對線上資料庫執行，agent 若無法連線，請在 README/PR 說明中明確標示為「待人工確認」，不要假設已比對。**
  3. 舊系統的 cron 曾把「到期保養」記成 `reporter_type='staff'` + `status='faulty'`，與真實故障無法可靠區分。README 註明此歷史資料語意不一致，不需自動轉換。
- **驗收**：在「舊版 schema」的測試資料庫上執行 migration 兩次皆成功；之後 `refresh_tokens` 可寫入、重複開單會得到 409、cron 能寫入 `system` 紀錄與 `pending_maint` 狀態。

---

## 2. P1：建議修

### T6. Refresh token：改為原子、失敗關閉，並對多分頁友善

- **位置**：`backend/internal/handlers/auth.go`（`generateTokens` 約 L75-78；`RefreshTokenHandler` 約 L186-198）
- **問題**：
  1. `generateTokens` 以 `_, _ = h.DB.Exec(INSERT ...)` 吞掉錯誤：寫入失敗時仍發出 token，但無法被追蹤或撤銷。
  2. 檢查 `revoked_at`（SELECT）與撤銷（UPDATE）是兩個獨立步驟，且 `err == nil && revokedAt != nil` 才攔截，**查不到列或查詢出錯時直接放行並簽發新 token（失敗開放）**。`jti` 缺失也直接放行。
  3. 兩個請求同時用同一個 refresh token 時，兩者都可能通過；而若後到的請求恰好在第一個完成撤銷之後才到，會被當成 reuse，**撤銷該使用者所有 token**，使多分頁使用者被同時登出。
- **做法**：
  1. `generateTokens`：INSERT 失敗要回傳錯誤，呼叫端回 500。
  2. refresh 流程改為單一原子語句：
     ```sql
     UPDATE refresh_tokens
        SET revoked_at = NOW()
      WHERE jti = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > NOW()
     RETURNING 1
     ```
     - 回 1 列 → 繼續簽發新 token。
     - 0 列 → 再查該 `jti`：**查無此列或已過期 → 401（失敗關閉）**；已被撤銷 → 若 `revoked_at` 在寬限秒數內（建議 10 秒，做成常數）→ 單純回 401、**不**撤銷整個使用者；超過寬限 → 才視為 reuse，撤銷該使用者所有未撤銷 token 並回 401。
     - refresh token 必須含 `jti`，缺少一律 401。
  3. （選做）在 cron 中清理已過期超過 N 天的 `refresh_tokens`。
- **驗收**：寫測試（需 PostgreSQL）：同一 refresh token 併發 N 次，恰好 1 次成功；刪除該列後 refresh 回 401；寬限內重送回 401 且其他 token 仍有效；超過寬限重送會撤銷全部；INSERT 失敗時登入回 500。

### T7. 改密碼／停用帳號時撤銷 refresh token

- **位置**：`user.go` `UpdateUser`
- **問題**：每次 refresh 都換發新的 24 小時 token，被盜的 refresh token 可無限續命；管理員重設密碼並不會使其失效。
- **做法**：重設密碼成功後，`UPDATE refresh_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`（與密碼更新放同一個交易）。角色變更會在下次 refresh 重新讀取，access token 內的舊角色最多存活 15 分鐘，請把這個取捨寫進 README。使用者刪除已有 `ON DELETE CASCADE`，確認即可。
- **驗收**：重設密碼後，舊 refresh token 立刻 401。

### T8. 排程可能把「故障」設備改回「待保養」

- **位置**：`maintenance.go:308-326`（`CheckAndCreateMaintenanceTasks`）
- **問題**：先查出到期設備，再逐筆 `INSERT ... ON CONFLICT DO NOTHING`，**接著無條件** `UPDATE equipments SET status='pending_maint'`。若在查詢與寫入之間有人報修，INSERT 因 unique index 被略過，但設備狀態從 `faulty` 被覆蓋成 `pending_maint`。
- **做法**：改成 set-based 並只更新真的插入成功的設備（清單第 13 點原本也建議）：
  ```sql
  WITH ins AS (
    INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved, resolve_note)
    SELECT e.lid, 'system', '【系統自動偵測】已達定期保養週期，請進行例行檢查。', false, ''
      FROM equipments e
     WHERE (CURRENT_DATE - e.last_maint_date) >= e.maint_interval
       AND e.retired_at IS NULL            -- 若實作 T9
    ON CONFLICT DO NOTHING
    RETURNING equipment_id
  )
  UPDATE equipments SET status = 'pending_maint'
   WHERE lid IN (SELECT equipment_id FROM ins);
  ```
  同時補上 `rows.Err()` 檢查。
- **驗收**：測試「已有 open 紀錄（faulty）的設備」在排程後狀態仍為 `faulty`。

### T9. 設備實務上刪不掉（需要 soft delete）

- **位置**：`equipment.go:275`、`init.sql:21`
- **問題**：`ON DELETE RESTRICT` 正確保護了歷史紀錄，但 cron 會替每台設備產生紀錄，等於幾乎所有設備都會因 FK 被擋（409），管理員無法下架設備。
- **做法**（產品決策，請依下列預設實作，若與需求衝突再回報）：
  - `equipments` 新增 `retired_at TIMESTAMPTZ NULL`（併入 migration）。
  - `DELETE /private/equipment`：無任何維修紀錄 → 硬刪；有紀錄 → 改為設定 `retired_at=NOW()`（並回 200，訊息註明為「已封存」）。
  - 所有列表、公開查詢、cron、stats 預設過濾 `retired_at IS NULL`；歷史紀錄頁與 Analytics 仍可顯示封存設備的紀錄。
  - `asset_code` 唯一性維持含已封存設備（避免編號重用造成歷史混淆），並在 README 註明。
  - 前端刪除按鈕文案改為「下架」並說明歷史會保留。
- **驗收**：對有紀錄的設備執行刪除 → 200 且該設備從列表與公開 API 消失、歷史紀錄仍在；排程不再為其建立任務。

### T10. 「最後一位 admin」檢查不是原子的

- **位置**：`user.go:101-107`、`199-206`
- **問題**：先 `COUNT(*)` 再 UPDATE/DELETE，兩位 admin 同時互相刪除或降級時兩邊都會通過，最後可能零 admin。
- **做法**：以交易包起來，先 `SELECT lid FROM users WHERE role='admin' FOR UPDATE` 鎖住 admin 列，再計數與寫入。
- **驗收**：併發測試（兩個交易互刪）最終至少剩一位 admin。

### T11. 仍回傳驗證與資料庫錯誤細節

- **位置**：`equipment.go:142`、`equipment.go:188`、`user.go:32`、`user.go:75`（`err.Error()` 直接回給 client）；`main.go` `/healthz` 在失敗時回傳 `err.Error()`。
- **做法**：回固定訊息（例如 `Invalid request payload`），細節只寫 `slog`；若要提供欄位層級錯誤，轉成 `{"field": "...", "reason": "..."}` 的白名單格式。`/healthz` 失敗時只回 `{"status":"unhealthy"}` 並記錄日誌。
- **驗收**：對上述端點送非法 JSON／缺欄位，回應不含 validator 內部欄位路徑或 Postgres 錯誤字串。

### T12. Swagger 未更新且公開暴露

- **位置**：`backend/docs/*`、`nginx.conf:17-25`、`main.go` 註解 `@host`
- **問題**：`docs/` 沒有重新產生，缺少 `/api/auth/logout`、`limit`/`offset`、新的 409 回應，仍含 `reporter_type` 欄位、`admin/admin` 範例與寫死的個人網域 `jupiterhsu.ddns.net`；而 nginx 把 `/swagger/` 公開轉發。
- **做法**：以與 `go.mod` 相符的版本執行 `swag init` 重新產生；`@host` 移除（或改由設定決定）；T3 新增的端點與 header 也要寫入註解；prod nginx 設定對 `/swagger/` 回 404（或加 basic auth），dev 保留。
- **驗收**：`grep -E "jupiterhsu|\"example\": \"admin\"" backend/docs` 無結果；swagger 有 `logout`、`limit`、`offset`、`X-Total-Count`；prod 設定下 `/swagger/` 不可公開存取。

### T13. 測試幾乎沒有覆蓋新邏輯

- **現況**：`middleware/auth_test.go`（InitJWT、AuthMiddleware、RoleRequired）品質尚可。`handlers/validation_test.go` 是用**自己寫的假 handler** 測 binding tag，沒有碰到真正的 handler。
- **做法**：用 `testcontainers-go` 或 CI 的 PostgreSQL service 對**真實 handler** 寫整合測試（`httptest` + 真實 `init.sql`）。最低限度涵蓋：
  - 公開報修併發送兩次 → 恰好一個 200、一個 409（驗證 partial unique index）。
  - resolve 兩次 → 200 後 409；不存在 → 404；`last_maint_date` 不被重複覆寫。
  - refresh 輪替與撤銷（T6）、改密碼後撤銷（T7）。
  - 刪除自己／降級自己 → 403；admin 併發互刪（T10）。
  - cron：已有 open 紀錄的設備不被改狀態（T8）。
  - T2 的 `InitJWT` production 護欄。
  - 分頁與 `X-Total-Count`、`/stats`（T3）。
- **驗收**：`go test ./...` 全過；說明如何在本機與 CI 執行（需要 Docker）。

### T14. 登入防暴力破解：補「每帳號」限制（選做）

- **現況**：nginx `login_limit` 在最後一個 commit 由 `5r/m` 放寬到 `30r/m burst=10`（合理，避免共用 NAT 誤傷），但這等於每 IP 每天可嘗試約 4 萬次，且分散式猜測不受限。
- **做法**：後端以帳號為單位計數（資料表或帶 TTL 的記憶體 map）：連續失敗 N 次（建議 5）暫時鎖定（建議 5-15 分鐘），**回應維持與一般失敗相同的 401 訊息**（不洩漏帳號是否存在），成功登入後歸零。鎖定判斷要在 bcrypt 比對之前，但仍須維持 dummy bcrypt 的 timing 一致。
- **驗收**：連續錯誤 N 次後，即使輸入正確密碼也會在鎖定期內被拒；鎖定結束後可正常登入。

---

## 3. P2：小項目（一次處理）

- [ ] **T15a** 密碼長度：`CreateUserRequest.Password` 的 `max=72` 是以**字元**計，bcrypt 上限是 **72 位元組**；中文密碼超過約 24 字會通過驗證卻在 `bcrypt.GenerateFromPassword` 失敗（`golang.org/x/crypto` v0.50.0 會回 `ErrPasswordTooLong`，依推演）→ 目前會是 500。在 handler 檢查 `len([]byte(pw)) <= 72`，不符回 400（`CreateUser`、`UpdateUser` 都要）。
- [ ] **T15b** `frontend/src/services/apiClient.ts` 的 `getStoredUser` 用 `atob` 解 JWT payload，`atob` 不處理 base64url（`-`/`_`）與非 ASCII；目前 claim 內容剛好安全，但日後加入中文欄位會壞。改成 base64url 轉換並以 `TextDecoder` 解碼。
- [ ] **T15c** ESLint 4 個錯誤：`react-hooks/set-state-in-effect` 於 `EquipmentList.tsx:54`、`MaintenanceTasks.tsx:27`、`UserManagement.tsx:53`（在 effect 內同步呼叫會 setState 的 fetch 函式）；`react-refresh/only-export-components` 於 `components/ui/Button.tsx:5`（把 `buttonVariants` 等常數移到獨立檔案）。修完讓 `npx eslint .` 為 0 錯誤。
- [ ] **T15d** `Dashboard.tsx` 的 `catch` 完全靜默：API 失敗時畫面顯示全 0，使用者無法分辨是「真的 0」或「載入失敗」。加上錯誤狀態顯示。
- [ ] **T15e** 大量 `alert()`（`UserManagement`、`EquipmentList`、`Analytics`、`MaintenanceTasks`）改為 toast 或行內錯誤。
- [ ] **T15f** `index.html` 的 `lang="en"` 改 `zh-TW`、`<title>` 更具描述性。
- [ ] **T15g** 移除殘留：`frontend/README.md`（Vite 範本）、`src/assets/hero.png`、`react.svg`、`vite.svg`（確認未被引用後再刪）。
- [ ] **T15h** 根目錄新增 `README.md`，包含：dev/prod 啟動方式（T1）、Production checklist（T2）、升級指南（T5）、**Security 章節只寫已實作項目**，並另列「已知限制與 Roadmap」：token 存 `localStorage`（XSS 風險，已用 CSP 緩解）、access token 內角色最多 15 分鐘延遲生效、refresh token 寬限期設計。
- [ ] **T15i** 啟動時的 `go h.CheckAndCreateMaintenanceTasks()` 已有 `TryLock` 保護，無需改動；但 `go func` 內若 panic 會使程序結束，建議加 `recover` 並記錄。

---

## 4. 不在範圍內（請勿順手更動）

- 不要把 token 改存 HttpOnly cookie（屬較大的架構變更，已列入 README 的已知取捨）。
- 不要變更 `/api/...` 現有路徑與回應 body 格式（T3 只新增 header 與 `/stats`）。
- 不要移除預設測試帳號與範例設備（只是改放到 dev 專用 seed，見 T2）。

---

## 5. 完成定義（Definition of Done）

1. `T1`–`T5` 全部完成並通過各自驗收。
2. 下列指令在 clean checkout 上全部通過：
   ```bash
   # backend
   cd backend && go build ./... && go vet ./... && go test ./...

   # frontend
   cd frontend && npm ci --legacy-peer-deps && npx tsc -b && npx eslint . && npm run build

   # compose
   docker compose config -q
   docker compose build
   docker compose -f docker-compose.yaml -f docker-compose.prod.yaml config -q   # 未設 JWT_SECRET 應失敗
   ```
3. 手動冒煙測試（dev）：登入（含連續輸錯）→ 後台各頁 → 報修 → 解決 → 登出。
4. PR 說明中明確列出：哪些項目已驗證、哪些無法驗證（例如沒有線上資料庫可比對 schema、Docker image tag 是否存在）。**不要宣稱沒跑過的東西「已通過」。**
