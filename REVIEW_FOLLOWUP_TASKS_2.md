# 修改任務單 #2：`database_final` 第二輪審查後續

> 對象：負責修改原始碼的 agent
> 審查基準：HEAD `b528277`（`fix(frontend): prevent infinite loading on repeated failed login attempts`）
> 前一份任務單：`REVIEW_FOLLOWUP_TASKS.md`（T1–T15）。本文件只列**第二輪審查後仍需處理的項目**，編號為 `R1`–`R12`、`S1`。
> 行號以審查基準為準，修改後可能位移。

---

## 0. 先讀這裡

### 0.1 測試用帳號與密碼（本文件的權威定義）

**所有任務、測試、README、腳本一律以本表為準。禁止自行更改本表的帳號或密碼。**

#### A. 開發環境種子帳號（`db/seed_dev.sql`，僅 dev 載入）

| 帳號 | 密碼 | 角色 | 存在的環境 |
|---|---|---|---|
| `admin` | `admin123456` | `admin` | **僅 dev**（`docker compose up` 預設） |
| `staff01` | `admin123456` | `staff` | **僅 dev** |

- 兩個帳號共用同一個 bcrypt hash：`$2a$10$ri5DZJg5np9NaV/Ghw49t.lnKSYV.9lJc/HcrC4YQmmFtwsUuWq0e`。審查者已用 bcrypt 驗證：此 hash 對應密碼 `admin123456`；`admin123`、`staff123` **不**符合。
- **正式環境（`docker-compose.prod.yaml`）不得出現這兩個帳號**，也不得載入 `seed_dev.sql`。
- README 目前把密碼寫成 `admin123` / `staff123`，是**錯的**（見 R1）。

#### B. 整合測試臨時帳號（由測試自己建立、自己刪除）

| 項目 | 規格 |
|---|---|
| 帳號命名 | `itest_<用途>_<UnixNano>`，例如 `itest_pwchange_1790000000000000000`（長度須 ≤ 32，必要時縮短用途字串） |
| 密碼 | `ItestPass#2026`（14 字元，符合 8–72 位元組規則） |
| 角色 | 依測試需要（`admin` 或 `staff`） |
| 生命週期 | 測試內建立，`defer` 清除（同時清除其 `refresh_tokens`） |
| 存在的資料庫 | 只存在於**測試資料庫**（見下表 C） |

> **鐵則：整合測試不得讀取、修改或刪除 A 表的種子帳號。** 目前的測試違反這條（見 R2）。

#### C. 整合測試專用資料庫（見 R2、R7）

| 項目 | 值 |
|---|---|
| 主機／埠 | `127.0.0.1` / `55432`（只綁本機，不對外） |
| 資料庫名稱 | `gets_test`（**名稱必須以 `_test` 結尾**，測試程式要檢查，否則拒絕執行） |
| DB 帳號／密碼 | `postgres` / `postgres` |
| 載入內容 | **只有** `db/init.sql`（schema），**不載入** `seed_dev.sql` |
| 測試用 `JWT_SECRET` | `integration_test_jwt_secret_32_chars_minimum_value`（僅限測試，`APP_ENV=development`） |

#### D. 正式環境

- 沒有預設帳號。第一位 admin 只能由 `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` 環境變數建立（見 R1）。
- 這兩個變數的值**不得寫入版控**，README 範例只能用佔位文字。

### 0.2 審查驗證了什麼、沒驗證什麼

**審查者已實際執行並通過：**
- 前端：`npm ci`、`tsc -b`（0 錯誤）、`eslint .`（0 錯誤）、`vite build`。
- 前端行為：以模擬後端對新舊 `apiClient` 跑 6 個情境，新版全數通過（連續輸錯密碼、5 個並發 401 只刷新 1 次、refresh 失敗登出、多分頁、無 refresh token）。
- SQL（PostgreSQL 16 實測）：`init.sql` 與 `seed_dev.sql` 可重複執行；部分唯一索引；cron 的 CTE（到期且故障的設備維持 `faulty`、已封存不被動到、重跑不重複開單）；`/stats` 三條查詢；migration 在舊 schema 上跑兩次皆成功、外鍵由 CASCADE 變 RESTRICT、遇重複未解決紀錄時中止並完整回滾。
- Go：`gofmt` 語法正確；自製輕量型別檢查（標準庫＋專案內部套件）無未使用 import／變數／未定義名稱。

**審查者無法驗證（你修改後請自己跑，並在 PR 說明誠實標註）：**
- `go build ./... && go vet ./... && go test ./...`（審查環境只有 Go 1.22，專案要 1.25，且無法下載 module）。
- gin／pgx／jwt API 是否使用正確（輕量檢查看不到）。
- Docker／Podman compose 的 `!override` 實際行為、`docker compose build`。
- 整合測試（需要 PostgreSQL）、真實瀏覽器行為、線上資料庫。

### 0.3 已正確，請勿改壞（回歸檢查清單）

- **T1** `nginx.prod.conf`（TLS、80→443、HSTS、`/swagger` 404、限流、Cloudflare real_ip）、`nginx.dev.conf`。
- **T2** `InitJWT` 的 production 護欄、`APP_ENV` 機制、`bootstrapAdmin` 環境變數建立 admin、種子與 schema 分離、prod override 取消 seed 掛載。
- **T4** `apiClient.ts` 的攔截器（排除 `auth/login` 與 `auth/refresh`、`finally` 重置、多分頁判斷、排隊 10 秒超時、base64url＋UTF-8 解碼）。
- **T5** `db/migrations/001_upgrade_from_beta_1_2.sql`（冪等、以 catalog 查約束、重複未解決紀錄時中止回滾）。
- **T6** refresh 原子輪替（單句 `UPDATE ... RowsAffected()`、查無列即 401、10 秒寬限期、reuse 撤銷全部、INSERT 失敗回 500、cron 清理過期 token）。
- **T7** 改密碼與撤銷 refresh token 同一交易。
- **T8** cron 的 set-based CTE。
- **T9** 軟刪除（列表、公開查詢、更新、cron、stats 皆過濾 `retired_at IS NULL`）。
- **T10** 最後一位 admin 的 `FOR UPDATE` 鎖定。
- **T11/T12** 不洩漏 `err.Error()`、Swagger 已重新產生且移除個人網域與 `admin/admin` 範例。
- **T15** 密碼 72 位元組檢查、ESLint 0 錯誤、無 `alert()`／`console.log`／`any`。

### 0.4 建議執行順序

`R2` → `R1` → `S1` → `R4` → `R6` → `R3` → `R5` → `R8` → `R9` → `R10` → `R7` → `R11` → `R12` → P2。
每完成一項就跑一次 §5 的驗證；**R2 完成前不要跑現有的整合測試**（會改掉 `staff01` 的密碼）。

---

## 1. P0：先處理

### R1. README 與實作不符（照做會失敗）

- **問題**（均已實測或實證）：
  1. **預設密碼寫錯**：README「開發環境預設帳號」寫 `admin123` / `staff123`，實際是 `admin123456`（兩個帳號相同）。
  2. **Bootstrap 說明是不存在的功能**：README §「正式環境初始管理員引導」描述「自動產生隨機臨時密碼並印在日誌」，但 `backend/main.go` 的 `bootstrapAdmin` 只讀 `BOOTSTRAP_ADMIN_USERNAME`／`BOOTSTRAP_ADMIN_PASSWORD`，**兩者任一未設就直接 `return`，沒有任何日誌**。照 README 部署正式環境：零帳號、日誌無提示、無法登入。
  3. **升級指南無法照做**：
     - 指令 `psql -U postgres -d gets` 的資料庫名稱錯誤（compose 預設是 `equipment_db`，且受 `DB_NAME`／`DB_USER` 控制）。
     - 缺少前一份任務單 T5 要求的**資料搬遷**說明：compose 把 `./postgres_data`（bind mount）換成 named volume `postgres_data`，既有部署照新 compose 啟動會得到**空資料庫**，看起來像資料全部遺失。
     - 缺少「以 `pg_dump -s` 對照線上 schema 與 `init.sql`」的步驟（`init.sql` 是在沒看到線上資料庫的情況下寫的）。
- **做法**：
  1. **README 開發帳號表**改為 §0.1-A 的值（`admin`／`admin123456`、`staff01`／`admin123456`），並註明「僅 dev，正式環境不存在」。
  2. **程式**：調整 `bootstrapAdmin`，先計算 `users` 筆數：
     - `users` 為空且環境變數未設定 → `slog.Warn("系統目前沒有任何使用者；請設定 BOOTSTRAP_ADMIN_USERNAME 與 BOOTSTRAP_ADMIN_PASSWORD 後重啟")`。
     - 環境變數存在但格式不合 → 現有的 `slog.Error` 保留。
     - 密碼長度下限與 `CreateUserRequest` 的 `min` 標籤**一致**（目前 bootstrap 允許 6、API 要求更高時需對齊）。
  3. **README Bootstrap 段落**改寫為實際行為：在 `.env` 設定兩個變數 → 啟動 → 登入 → **立即改密碼並從 `.env` 移除這兩個變數**。範例值只能用佔位（`<管理員帳號>`、`<至少 8 字元的強密碼>`）。`.env.example` 以註解列出這兩個變數。
  4. **README 升級指南重寫**，至少包含下列步驟（指令需用變數，不可寫死 `gets`）：
     1. 備份：舊環境 `pg_dump -Fc` 輸出到檔案。
     2. 確認現有資料庫名稱與使用者（`.env` 的 `DB_NAME`、`DB_USER`）。
     3. 以 `pg_dump -s` 倒出線上 schema，與 `db/init.sql` 比對；**差異以線上為準修正 `init.sql`**。若 agent 無法連線線上庫，PR 說明必須標示「待人工比對」，不得宣稱已比對。
     4. 資料搬遷：舊 bind mount → 新 named volume。建議流程：啟動新 stack（prod，不含 seed）→ `pg_restore --clean --if-exists --no-owner` 還原備份 → 依序執行 `db/migrations/*.sql`。
     5. 執行 migration 的指令改為在 db 容器內使用其環境變數，例如：
        `docker compose exec -T db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < db/migrations/001_upgrade_from_beta_1_2.sql`
     6. 驗證清單：能以 bootstrap 或既有 admin 登入、設備與維修紀錄筆數與備份一致。
     7. 若 agent 有可用的 Docker，請在拋棄式環境演練整個流程並在 PR 貼出輸出；否則標註「未演練」。
- **驗收**：
  - README 內所有帳號／密碼與 §0.1 逐字相符（`grep -n "admin123" README.md` 只應出現 `admin123456`）。
  - 在乾淨 prod 環境（無 seed）只設 `BOOTSTRAP_ADMIN_*` 啟動 → 日誌顯示「已成功建立初始管理員帳號」→ 可登入。
  - 在乾淨 prod 環境**不設** `BOOTSTRAP_ADMIN_*` 啟動 → 日誌出現 WARN 提示。
  - 升級指南的每個指令都能直接複製執行（不含寫死的資料庫名稱）。

### R2. 整合測試會永久改掉種子帳號 `staff01` 的密碼，並改為使用隔離測試庫

- **問題**：
  - `backend/internal/handlers/integration_test.go` 的 `TestIntegration_PasswordChange_RevokesRefreshTokens` 以 `SELECT lid FROM users WHERE role = 'staff' LIMIT 1` 取到種子帳號 `staff01`，並透過 API 把密碼改成 `newpassword123456`，**結束後沒還原**。跑過一次測試，§0.1-A 的 `staff01`／`admin123456` 就失效。
  - 其他測試也依賴種子資料（`SELECT ... role = 'admin' LIMIT 1`）。
  - `TestIntegration_AdminDemotion_Protection` 只在「剛好只有 1 位 admin」時才有斷言，且使用「已刪除使用者」的 token，不是真實情境。
  - 測試連到 `DB_HOST` 所指的**任何**資料庫，沒有安全檢查。
  - `getTestPool` 預設密碼 `change_this_to_a_secure_password` 與 compose 預設 `postgres` 不一致。
- **做法**：
  1. **新增 `docker-compose.test.yaml`**：一個 `db-test` 服務（`postgres:15-alpine`），規格依 §0.1-C：`POSTGRES_DB=gets_test`、`POSTGRES_USER=postgres`、`POSTGRES_PASSWORD=postgres`、`ports: ["127.0.0.1:55432:5432"]`、資料放 `tmpfs`、只掛載 `db/init.sql`（**不**掛 `seed_dev.sql`）。
  2. **`getTestPool` 重寫**：
     - 預設值對齊 §0.1-C（`127.0.0.1`、`55432`、`postgres`／`postgres`、`gets_test`）。
     - **若資料庫名稱不以 `_test` 結尾 → `t.Fatalf` 拒絕執行**（防止誤連 dev／prod）。
     - 未設定 `DB_HOST` 時：預設 `t.Skip`，但訊息要明確指出「整合測試未執行」；若設定 `REQUIRE_DB_TESTS=1` 則改為 **失敗**（供 CI 使用，避免靜默通過）。
  3. **測試輔助函式**：新增 `createTestUser(t, pool, role) (lid, username string)`，建立 §0.1-B 規格的臨時帳號（密碼 `ItestPass#2026`，以 bcrypt 雜湊後 INSERT），並以 `t.Cleanup` 刪除（同時刪除其 `refresh_tokens`）。所有需要 admin／staff 的測試一律用它，**不得**查詢 `users` 表取現成帳號。
  4. **`TestIntegration_PasswordChange_RevokesRefreshTokens`**：改為建立臨時 admin 與臨時 staff，用臨時 admin 改臨時 staff 的密碼。
  5. **`TestIntegration_AdminDemotion_Protection`**：重寫為「測試庫中只有 2 位臨時 admin」的真實情境（見 R7 的併發測試）。
  6. README 新增「執行整合測試」：
     ```bash
     docker compose -f docker-compose.test.yaml up -d db-test
     cd backend
     DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=gets_test \
     JWT_SECRET=integration_test_jwt_secret_32_chars_minimum_value REQUIRE_DB_TESTS=1 \
     go test ./... -v -count=1
     docker compose -f docker-compose.test.yaml down -v
     ```
- **驗收**：
  - 在 dev 環境（含 seed）先確認 `admin`／`staff01` 可登入 → 對**測試庫**跑完整套整合測試 → 再確認 dev 環境的兩個帳號仍可登入（由 S1 腳本檢查）。
  - 把 `DB_NAME` 改成 `equipment_db` 跑測試 → 測試立即失敗並指出名稱不符。
  - 不設 `DB_HOST` 跑 `go test ./... -v` → 輸出明確顯示整合測試被 skip；加上 `REQUIRE_DB_TESTS=1` → 失敗。
  - `grep -n "role = 'staff' LIMIT 1\|role = 'admin' LIMIT 1" backend` 無結果。

---

## 2. P1：建議修

### R3. 設備搜尋只搜「目前這一頁」

- **位置**：`frontend/src/pages/admin/EquipmentList.tsx:224`（`equipments.filter(...)`）
- **問題**：分頁後每頁 20 筆，搜尋只過濾目前這頁。輸入其他頁設備的資產編號會顯示「找不到符合搜尋條件的器材」，但設備明明存在。
- **做法**：
  1. 後端 `GET /api/private/equipments` 新增選用參數 `q`：對 `name`、`asset_code`、`location` 做不分大小寫的部分比對（`ILIKE`）。**必須跳脫 `%`、`_`、`\`**；`q` 長度上限 50（超過回 400）；`X-Total-Count` 須套用相同篩選；排序維持 `created_at DESC, lid`。
  2. 前端：搜尋字串 debounce 300 ms 後傳 `q`，並把頁碼重設為 1；移除前端 `filter`。
  3. 更新 swagger 與 `types.ts`。
- **驗收**：灌入 60 台設備，編號 `Z-059` 位於第 3 頁；在第 1 頁搜尋 `Z-059` 能找到；搜尋 `%` 不會回傳全部（被當一般字元）；`X-Total-Count` 隨搜尋結果改變。

### R4. `/stats` 近六個月趨勢在每月 29–31 日會錯亂

- **位置**：`backend/internal/handlers/equipment.go` `GetStats`（`now.AddDate(0, -(5 - i), 0)`）
- **問題**（已實測）：Go 的 `AddDate` 在日期溢位時會進位。例如 2026-10-31 產生 `[2026-05, 2026-07, 2026-07, 2026-08, 2026-10, 2026-10]`（缺 6、9 月，且重複）。此問題原本就存在於舊前端的 `setMonth`，搬到後端時沒修。**下次觸發日期：2026-10-31。**
- **做法**：抽出純函式 `buildTrendMonths(now time.Time) []trendItem`，以**當月 1 日**為基準再 `AddDate`：
  ```go
  first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
  d := first.AddDate(0, -(5 - i), 0)
  ```
- **驗收（單元測試，不需資料庫）**，`now` 使用 `Asia/Taipei`：

  | now | 預期 `monthKey` 序列 |
  |---|---|
  | 2026-10-04 | 2026-05, 06, 07, 08, 09, 10 |
  | 2026-10-31 | 2026-05, 06, 07, 08, 09, 10 |
  | 2026-08-31 | 2026-03, 04, 05, 06, 07, 08 |
  | 2026-03-30 | 2025-10, 11, 12, 2026-01, 02, 03 |
  | 2026-05-31 | 2025-12, 2026-01, 02, 03, 04, 05 |

  `name` 欄位為 `N月`（例如 `10月`）。

### R5. 刪除或解決「最後一頁的最後一筆」後，畫面卡住

- **位置**：`EquipmentList.tsx`（刪除後 `fetchEquipments(page)`）、`MaintenanceTasks.tsx`（解決後 `fetchTasks(page)`）、`UserManagement.tsx`（刪除後重抓）
- **問題**：操作完仍用原頁碼重抓。若該頁變空，且總頁數降為 1（分頁器因 `totalPages > 1` 而隱藏），畫面顯示「目前沒有資料」，使用者無法返回。例如 21 筆資料在第 2 頁刪掉第 21 筆。
- **做法**：取得 `X-Total-Count` 後計算 `maxPage = max(1, ceil(total / pageSize))`，若目前頁碼 > `maxPage` 則 `setPage(maxPage)`（由既有的 effect 重抓）。順便處理 `handleAddSubmit` 成功後「`fetchEquipments(1)` 加 `setPage(1)`」造成的重複請求。
- **驗收**：21 筆資料，在第 2 頁刪除唯一一筆 → 自動回到第 1 頁並顯示 20 筆；三個頁面都要驗。

### R6. CSV 匯出的公式注入（匿名民眾輸入會進入管理員的 Excel）

- **位置**：`frontend/src/pages/admin/Analytics.tsx` `handleExportCSV`（約 L44–100）
- **問題**：只跳脫了雙引號。`description` 可由匿名民眾經公開報修 API 填寫（最多 500 字），以 `=`、`+`、`-`、`@` 開頭的內容在 Excel 會被當成公式執行。
- **做法**：新增 `src/lib/csv.ts` 的 `csvCell(value: unknown): string`：
  1. 轉成字串；
  2. 若開頭為 `=`、`+`、`-`、`@`、Tab（`\t`）或 CR（`\r`），在前面加單引號 `'`；
  3. 將 `"` 換成 `""`；
  4. 以雙引號包起來。
  所有文字欄位（含 `lid`、設備名稱、資產編號、描述、備註）都經過 `csvCell`。
- **驗收**：公開報修送出描述 `=HYPERLINK("http://example.com","x")` → 匯出的 CSV 該儲存格內容為 `'=HYPERLINK(...)`（以文字編輯器開啟檢查）；一般內容不受影響；含逗號與換行的描述仍正確。

### R7. 補齊後端測試缺口

- **現況**：整合測試覆蓋不足，且以下任務單 T13 要求的項目未實作。`/stats` 測試只驗長度為 6，抓不到 R4。
- **做法**（皆使用 R2 的測試庫與 `createTestUser`）：

  | 測試 | 斷言 |
  |---|---|
  | refresh 併發 | 同一 refresh token 併發 10 次 → **恰好 1 次 200**，其餘 401 |
  | refresh 失敗關閉 | 直接刪除該 `jti` 的資料列後 refresh → 401 |
  | refresh reuse | 以 SQL 把 `revoked_at` 改成 > 10 秒前，再用舊 token → 401，且該使用者其他未撤銷 token 全被撤銷 |
  | 兩位 admin 互刪 | 測試庫恰有 2 位臨時 admin，兩者**同時**互刪 → 恰好 1 個成功、1 個 403，最後 admin 數 ≥ 1 |
  | cron 不蓋 faulty | 已有未解決紀錄且 `status='faulty'` 的到期設備，呼叫 `h.CheckAndCreateMaintenanceTasks()` 後狀態仍為 `faulty`；封存設備不被建立任務 |
  | 分頁 | 建立 > 100 筆設備，`X-Total-Count` 正確；相同 `created_at` 的資料換頁不重複不遺漏 |
  | 軟刪除 | 封存後：列表、公開查詢、`PostMaintenanceRecord` 皆視為不存在；歷史紀錄仍在 `/private/maintenance-records` |
  | 密碼長度 | 73 位元組密碼（含 25 個中文字）建立使用者 → 400，不是 500 |
  | 趨勢月份 | R4 的表格測試 |
  | InitJWT | 已有，保留 |

- **驗收**：`REQUIRE_DB_TESTS=1 go test ./... -count=1` 全過；README 有 R2 的執行說明；PR 貼出測試輸出。

### R8. 資產編號唯一性：README 與實作不一致（建議採 README 的說法）

- **現況**：README 升級段落宣稱「資產編號唯一性改為條件式索引（`WHERE retired_at IS NULL`），退役設備不佔用編號」。實際上 `init.sql` 是 `asset_code VARCHAR(50) UNIQUE NOT NULL`，migration 也沒有這項變更，**封存設備仍佔著編號**；管理員封存舊設備後無法用同編號登錄替代品，且收到的 409 訊息指向一個「看不見」的設備。公開查詢本來就過濾封存設備。
- **做法（選項 A，建議）**：
  1. `init.sql`：移除欄位層級的 `UNIQUE`，改為 `CREATE UNIQUE INDEX IF NOT EXISTS uq_equipments_asset_code_active ON equipments (asset_code) WHERE retired_at IS NULL;`。
  2. 新增 `db/migrations/002_asset_code_partial_unique.sql`（冪等）：以 catalog 查出 `equipments` 上針對 `asset_code` 的 unique 約束並刪除，再建立上述索引；放在交易內。
  3. `db/seed_dev.sql`：`ON CONFLICT (asset_code) DO NOTHING` 改為 `ON CONFLICT (asset_code) WHERE retired_at IS NULL DO NOTHING`（部分索引推斷需要附上 predicate）。
  4. 後端 `PostEquipment`／`UpdateEquipment` 的 `23505` → 409 處理維持。
  5. README 升級指南寫明依序執行 001、002。
- **做法（選項 B，不建議）**：維持現狀，修正 README 的錯誤描述，並把 409 訊息改成「編號已被使用（可能為已下架設備）」。
- **驗收（選項 A）**：封存設備 X 後，以相同編號建立新設備 → 200；公開查詢該編號 → 回傳新設備；同時存在兩台**未封存**同編號設備 → 409；`init.sql` 與 migration 後的 schema 在此項上一致；`seed_dev.sql` 重複執行不報錯。

### R9. refresh 輪替：先撤銷舊 token，後續失敗會燒掉 token

- **位置**：`backend/internal/handlers/auth.go` `RefreshTokenHandler`（約 L197–265）
- **問題**：原子 `UPDATE` 撤銷舊 token 之後，才查使用者角色並簽發、寫入新 token。若中途失敗（資料庫抖動、使用者剛好被刪），舊 token 已作廢，使用者被迫重新登入；前端重送同一 token 會落在寬限期內得到 401。
- **做法**：「撤銷舊 token → 查使用者角色 → 寫入新 token」放在**同一個交易**；任一步驟失敗就 rollback，舊 token 保持有效。將 `generateTokens` 改為接受只需要 `Exec` 的介面（`pgx.Tx` 與 `*pgxpool.Pool` 皆滿足），登入流程繼續用連線池。
- **驗收**：既有 refresh 測試全過；新增測試以注入失敗（例如將 `refresh_tokens` 暫時改名造成 INSERT 失敗，或使用會失敗的 Exec 包裝）驗證：失敗時舊 refresh token 仍可再次使用。

### R10. 進入 `/login` 就清空登入狀態

- **位置**：`frontend/src/pages/auth/Login.tsx`（`useEffect(() => { handleForceLogout(); }, [])`）
- **問題**：每次載入登入頁就清除 `localStorage` 的兩個 token。`localStorage` 在分頁間共用，所以在新分頁開啟 `/login` 會讓**所有分頁**被登出，與 T4 的多分頁設計矛盾；且只清本機、不撤銷伺服端 token。apiClient 已修好（實測連續輸錯密碼不再卡住），這行已不必要。
- **做法**：移除該 `useEffect`。若仍需重置刷新狀態，在 `apiClient.ts` 匯出只重置 `isRefreshing`／`failedQueue` 的 `resetRefreshState()`（不碰 token）。（選做）已登入者進入 `/login` 時導向 `/admin`。
- **驗收**：分頁 A 已登入，分頁 B 開 `/login` → 分頁 A 繼續正常使用；連續輸錯密碼 3 次，每次都顯示錯誤且按鈕恢復（S1 或手動）。

### R11. 驗證 compose 的 `!override`

- **位置**：`docker-compose.prod.yaml`（`ports: !override`、`volumes: !override`）
- **問題**：`!override` 需要 Docker Compose ≥ 2.24.4；README 同時寫 `podman compose`，是否支援取決於底層 provider（docker-compose 或 podman-compose）。審查者無法驗證。
- **做法**：
  1. 執行並把關鍵輸出貼到 PR：
     ```bash
     docker compose -f docker-compose.yaml -f docker-compose.prod.yaml config
     ```
     預期：frontend 的 ports 只有 `80:80`、`443:443`；volumes 為 `nginx.prod.conf` 與 `./ssl`；**db 的 volumes 沒有 `seed_dev.sql`**；backend 的 `APP_ENV=production`。
  2. 不設 `JWT_SECRET`、`DB_PASSWORD` 時，上述指令應**失敗**並顯示 `:?` 的錯誤訊息。
  3. dev：`docker compose config` 的 db volumes 應包含 `01-init.sql` 與 `02-seed_dev.sql`。
  4. README 註明最低版本（Docker Compose ≥ 2.24.4）；若無法驗證 podman，寫明「未驗證」並提供不使用 `!override` 的替代方式（例如 dev 專用 override 檔掛 seed）。
- **驗收**：PR 附上三項指令的實際輸出；無法執行者標註「未驗證」。

### R12. README 其他不準確之處

逐項修正（`R1` 已處理的不重複）：

1. **護欄說明**：README 寫會擋 `secret_key` 與弱 `DB_PASSWORD`。實際只擋 `please_generate`、`change_this`、與範例完全相同的值；`DB_PASSWORD` 只檢查「有沒有設」（compose 的 `:?`）。二選一：修正文字，或補上程式（建議：`production` 下 `DB_PASSWORD` 含 `change_this`／等於 `postgres` 時拒絕啟動）。
2. **`JWT_SECRET` 範例**：README 範例是看起來真實的 64 字元 hex，可通過護欄且公開在 repo。改成 `<請執行：openssl rand -hex 32>`。
3. **「驗證中介軟體 fail-closed」**：此說法只適用 **refresh 流程**。access token 是無狀態 JWT，撤銷或改密碼後最多仍可用 **15 分鐘**。README「安全性功能架構」與「已知限制」都要如實寫出。
4. **技術描述**：「`RETURNING 1`」→ 實作是單句 `UPDATE` 搭配 `RowsAffected()`；「`SELECT count(*) ... FOR UPDATE`」→ 實作是 `SELECT lid ... FOR UPDATE` 後在程式計數（`COUNT(*) ... FOR UPDATE` 在 PostgreSQL 是不合法的）；「死鎖窘境」用語不精確。
5. **`DOMAIN`**：`.env` 範例中的 `DOMAIN` 未被任何設定使用（`nginx.prod.conf` 用 `server_name _`）。移除，或說明僅供參考。
6. **migration 說明**：README 稱新增了「查詢索引」，001 並沒有建立；刪除該句。001 會把 `maint_interval < 1` **靜默改成 30**，請在 migration 以 `RAISE NOTICE` 列出被修改的筆數，並寫進 README。
7. **兩種 constraint 名稱**：全新安裝（`init.sql`，自動命名如 `equipments_status_check`）與升級後（migration，`chk_*`）名稱不同。於 README 註明，並確保日後 migration 一律以定義查詢，不依賴名稱。
8. **Cloudflare IP 清單**是寫死的，README 註明需定期對照官方清單。
9. **Security 章節**只寫已實作項目；「已知限制」補上：token 存 `localStorage`、access token 15 分鐘延遲生效、refresh 寬限期設計、無每帳號登入鎖定（見 P2）。

- **驗收**：逐條對照程式碼，README 中不得有與實作矛盾的敘述；PR 說明列出每條的處理方式（修文字或修程式）。

---

## 3. 驗證與腳本

### S1. Dev 冒煙腳本（鎖定 §0.1 的測試帳號）

- **新增** `scripts/smoke_dev.sh`（需 `curl`，輸出 PASS／FAIL，任一 FAIL 則 exit 1），針對 `http://localhost:${HTTP_PORT:-8000}`：

  | 檢查 | 預期 |
  |---|---|
  | `POST /api/auth/login` `admin` / `admin123456` | 200，含 `access_token` 與 `refresh_token` |
  | `POST /api/auth/login` `staff01` / `admin123456` | 200 |
  | `POST /api/auth/login` `admin` / `admin123` | **401**（確認 README 舊值確實無效） |
  | `POST /api/auth/login` `staff01` / `staff123` | **401** |
  | admin token → `GET /api/private/stats` | 200 |
  | staff token → `GET /api/private/users` | **403** |
  | 無 token → `GET /api/private/equipments` | 401 |
  | `GET /api/public/equipment?asset_code=RUN-001` | 200 |
  | `GET /healthz` | 200 |

- 在 README「快速上手」引用此腳本。
- **使用時機**：每次改完、**以及整合測試跑完之後**都要跑一次，確保種子帳號沒被破壞。
- **驗收**：全新 `docker compose up -d` 後腳本全 PASS；整合測試跑完後再跑一次仍全 PASS。

---

## 4. P2：小項目（一次處理）

- [ ] **P2-1** 刪除根目錄舊的 `nginx.conf`：沒有任何檔案引用，且與 `nginx.dev.conf`、`nginx.prod.conf` 都不同，容易被誤改。
- [ ] **P2-2** 執行 `gofmt -w`：`internal/handlers/equipment.go`、`user.go`、`integration_test.go` 目前沒有通過 `gofmt -l`。
- [ ] **P2-3** `DeleteEquipment`：先查「有無維修紀錄」再 `DELETE`，兩步之間若有人報修，`DELETE` 會因外鍵得到 `23503` 並回 500。遇 `23503` 時改走封存（`retired_at = NOW()`）。補整合測試。
- [ ] **P2-4** Swagger：`GetMaintenanceRecords` 缺 `@Header 200 {integer} X-Total-Count`；補上後重新執行 `swag init`。`GetStats`、`q` 參數（R3）也要同步。
- [ ] **P2-5** 刪除 `frontend/src/assets/vite.svg`（確認 `grep -rn vite.svg frontend/src frontend/index.html` 無引用）。
- [ ] **P2-6** 前端 `EquipmentList`／`MaintenanceTasks`／`UserManagement` 中「`fetchX` 函式」與「`useEffect` 內的 `load`」邏輯重複（為了通過 `react-hooks/set-state-in-effect`）。改為單一資料取得函式（例如自訂 hook）。
- [ ] **P2-7**（選做）登入的「每帳號連續失敗鎖定」（前一份任務單 T14）：連續失敗 5 次暫時鎖定 5–15 分鐘，回應與一般失敗相同（不洩漏帳號是否存在）。
- [ ] **P2-8** `GetStats`：`trendRows.Scan` 的錯誤被靜默忽略（`err == nil` 才寫入）、各 `rows` 缺 `rows.Err()` 檢查；補上並記錄日誌。
- [ ] **P2-9** `bootstrapAdmin` 與 `generateTokens` 的 `slog` 訊息混用中英文，統一語言（選做）。

---

## 5. 不在範圍內（請勿順手更動）

- **不得變更 §0.1 的測試帳號與密碼**；不得移除 dev 的種子帳號與範例設備。
- 不要把 token 改存 HttpOnly cookie（較大的架構變更，已列入 README「已知限制」）。
- 不要變更既有 `/api/...` 路徑與回應 body 格式（可新增參數與 header）。
- 不要把 `seed_dev.sql` 的內容搬回 `init.sql`。

---

## 6. 完成定義（Definition of Done）

1. R1、R2 完成並通過各自驗收；其餘 P1 任務逐項完成或在 PR 說明中明確列出「未做／原因」。
2. 下列指令在乾淨 checkout 全數通過：
   ```bash
   # 後端（需 Go 1.25）
   cd backend && gofmt -l . && go build ./... && go vet ./... && go test ./... -count=1

   # 後端整合測試（隔離測試庫，見 R2）
   docker compose -f docker-compose.test.yaml up -d db-test
   cd backend && DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=postgres DB_PASSWORD=postgres \
     DB_NAME=gets_test JWT_SECRET=integration_test_jwt_secret_32_chars_minimum_value \
     REQUIRE_DB_TESTS=1 go test ./... -v -count=1
   docker compose -f docker-compose.test.yaml down -v

   # 前端
   cd frontend && npm ci --legacy-peer-deps && npx tsc -b && npx eslint . && npm run build

   # compose
   docker compose config -q
   docker compose build

   # dev 冒煙（整合測試前後各跑一次）
   docker compose up -d && bash scripts/smoke_dev.sh
   ```
3. 手動冒煙（dev，使用 §0.1-A 帳號）：以 `admin`／`admin123456` 登入 → 後台各頁（含搜尋、分頁、CSV 匯出）→ 報修 → 解決 → 登出；連續輸錯密碼 3 次；開第二個分頁到 `/login` 後第一個分頁仍可使用。
4. PR 說明必須誠實列出：
   - 哪些指令已實際執行、哪些**未能執行**（例如無 Go 1.25、無 Docker、無線上資料庫可比對 schema）。
   - **不得宣稱沒跑過的東西「已通過」。**
