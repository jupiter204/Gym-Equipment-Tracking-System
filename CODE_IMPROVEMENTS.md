# 原始碼改進清單

> 範圍：`backend/`（Go 1.25 / Gin / pgx）、`frontend/`（React / TypeScript / Vite）、`docker-compose.yaml`、`nginx.conf`
> 方式：靜態閱讀原始碼，未實際執行。標示「待確認」的項目需要你對照 DB schema 或實測驗證。
> 優先級：🔴 高（資安／正確性）、🟡 中（工程品質）、🟢 低（整潔）

---

## 總覽

| # | 項目 | 優先級 | 預估時間 |
|---|------|:---:|:---:|
| 1 | JWT secret 有預設值 | 🔴 | 10 分 |
| 2 | `init()` 早於 `godotenv.Load()` | 🔴 | 10 分（與 1 一起修） |
| 3 | 公開報修端點信任 client 傳的 `reporter_type`、無長度上限 | 🔴 | 15 分 |
| 4 | 報修流程的競態條件 | 🔴 | 20 分 |
| 5 | Refresh Token 流程缺驗證 | 🔴 | 20 分 |
| 6 | 登入的 timing 帳號枚舉 | 🔴 | 10 分 |
| 7 | 登入端點缺少限流與 XFF 可偽造 | 🔴 | 15 分 |
| 8 | 管理員可刪除／降級自己、角色與密碼未驗證 | 🔴 | 20 分 |
| 9 | Resolve 未檢查是否已解決 | 🔴 | 5 分 |
| 10 | 新增／修改設備未處理重複 `asset_code` | 🟡 | 10 分 |
| 11 | 刪除設備會連帶硬刪維修歷史 | 🟡 | 視設計 |
| 12 | Cron 時區與 `CURRENT_DATE` | 🟡 | 10 分 |
| 13 | 保養到期與故障狀態語意混淆 | 🟡 | 30 分 |
| 14 | 錯誤處理與 API 回應不一致 | 🟡 | 30 分 |
| 15 | 無分頁、無測試、`handlers.go` 過大 | 🟡 | 較長 |
| 16 | `main.go`／Dockerfile／compose 強化 | 🟡 | 30 分 |
| 17 | Token 存放在 localStorage | 🟡 | 視設計 |
| 18 | 前端：route guard、型別、殘留 | 🟡 | 30 分 |

---

## 🔴 高優先：資安與正確性

### 1. JWT secret 有預設值

**位置**：`backend/internal/middleware/auth.go`

```go
secret := os.Getenv("JWT_SECRET")
if secret == "" {
    secret = "default_secret_key_for_development"
}
```

**問題**：忘記設定環境變數時，服務仍會啟動，且任何人都能用這個公開在原始碼中的字串偽造 admin token。

**建議**：缺少就直接啟動失敗，並要求最小長度。

```go
// middleware/auth.go
var jwtKey []byte

func InitJWT() error {
    secret := os.Getenv("JWT_SECRET")
    if len(secret) < 32 {
        return errors.New("JWT_SECRET 未設定或長度不足 32 字元")
    }
    jwtKey = []byte(secret)
    return nil
}
```

### 2. `init()` 早於 `godotenv.Load()`

**位置**：`middleware/auth.go` 的 `init()` 與 `main.go` 的 `godotenv.Load()`

**問題**：Go 的 package `init()` 會在 `main()` 之前執行，所以本機用 `.env` 開發時，`JWT_SECRET` 在被讀取的當下還沒載入，永遠落到預設值。Docker 環境因為 compose 直接注入環境變數所以沒問題，這類「只在本機出錯」的 bug 很難察覺。

**建議**：移除 `init()`，改在 `main()` 內、`godotenv.Load()` 之後明確呼叫 `InitJWT()`（與第 1 點一起處理）。

```go
// main.go
_ = godotenv.Load()
if err := middleware.InitJWT(); err != nil {
    slog.Error("初始化失敗", "err", err)
    os.Exit(1)
}
```

### 3. 公開報修端點信任 client 輸入

**位置**：`models.MaintenanceRequest`、`handlers.PostMaintenanceRecord`

**問題**：
- `reporter_type` 由 client 傳入，匿名使用者可以填 `staff` 冒充內部人員（DB 的 CHECK 只能擋非法值，擋不了合法但偽造的值）。
- `description` 沒有長度上限，公開端點可被塞入大量資料。
- 沒有限制 request body 大小。

**建議**：公開端點由 server 固定來源，並限制長度。

```go
// models
type MaintenanceRequest struct {
    EquipmentID string `json:"equipment_id" binding:"required,uuid"`
    Description string `json:"description"  binding:"required,min=1,max=500"`
}

// handler 內
c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10) // 8 KB
const reporterType = "public"
```

### 4. 報修流程的競態條件

**位置**：`PostMaintenanceRecord`

**問題**：先 `SELECT EXISTS(未解決紀錄)` 再 `INSERT`。兩個請求同時進來（例如使用者連點兩次）時，都會看到「沒有未解決紀錄」而各自新增，造成同一設備有兩筆未解決報修。交易的預設隔離等級（READ COMMITTED）擋不住這個情況。

**建議**：用資料庫約束保證，而不是靠應用層檢查。這也是很適合寫進資料庫課程報告的設計點。

```sql
CREATE UNIQUE INDEX uq_one_open_record_per_equipment
ON maintenance_records (equipment_id)
WHERE is_resolved = false;
```

```go
if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
    c.JSON(http.StatusConflict, gin.H{"error": "An unresolved record already exists"})
    return
}
```

原本的 SELECT 可保留作為友善的提前回應，但正確性由 index 負責。`CheckAndCreateMaintenanceTasks` 同樣受這個 index 保護。

### 5. Refresh Token 流程

**位置**：`handlers.RefreshTokenHandler`、`generateTokens`、`AuthMiddleware`

**問題**：
1. `RefreshTokenHandler` 的 keyfunc 沒有檢查簽章演算法。
2. `claims["userUUID"].(string)` 是無檢查的型別斷言，claim 缺失時會 panic。
3. 使用者已被刪除時，`QueryRow` 回 `ErrNoRows`，卻回傳 500，應為 401。
4. `AuthMiddleware` 沒有確認這是 access token。目前 refresh token 拿去呼叫私有 API 會被擋，純粹是因為它沒有 `role` claim，屬於巧合。
5. 沒有 `iat`、`jti`、`iss` 等標準 claim。
6. refresh token 無狀態，無法撤銷，登出只是前端清除 localStorage。

**建議（最低限度）**：

```go
token, err := jwt.Parse(req.RefreshToken, keyFunc,
    jwt.WithValidMethods([]string{"HS256"}),
    jwt.WithExpirationRequired(),
)
// ...
userUUID, ok := claims["userUUID"].(string)
if !ok { /* 401 */ }

// access token 加上 "type": "access"，middleware 中檢查
if claims["type"] != "access" { /* 401 */ }

// ErrNoRows 回 401，其他錯誤才回 500
```

**建議（進階，做完才能宣稱「可撤銷」）**：新增 `refresh_tokens(jti, user_id, expires_at, revoked_at)` 資料表，每次 refresh 做 rotation（舊的作廢、發新的），偵測到已作廢的 token 被重複使用時，撤銷該使用者的整個 token family。並新增 `POST /api/auth/logout`。

### 6. 登入的 timing 帳號枚舉

**位置**：`handlers.AuthenticateUser`

**問題**：帳號不存在時直接回 401，沒有執行 bcrypt；帳號存在但密碼錯誤時要跑一次 bcrypt（約數十到上百毫秒）。攻擊者可從回應時間差判斷帳號是否存在。另外，`QueryRow` 的任何錯誤（包含 DB 連線失敗）都被當成 401，會掩蓋真正的故障。

**建議**：

```go
// 啟動時產生一次
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy"), bcrypt.DefaultCost)

// 查無使用者時
if errors.Is(err, pgx.ErrNoRows) {
    bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
    c.JSON(401, ...)
    return
}
if err != nil { /* 500 並記錄 */ }
```

### 7. 登入端點缺少限流，且 XFF 可被偽造

**位置**：`nginx.conf`、`main.go`

**現況**：nginx 已對 `/api/public/` 設定 `limit_req`，這點做得很好；但真正會被暴力破解的 `/api/auth/login` 沒有任何限流。

**建議一（最快）：在 nginx 加一個 login 專用 zone**

```nginx
limit_req_zone $binary_remote_addr zone=login_limit:10m rate=5r/m;

location = /api/auth/login {
    limit_req zone=login_limit burst=3 nodelay;
    limit_req_status 429;
    proxy_pass http://backend:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $remote_addr;   # 覆蓋而非追加
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

**建議二：同時修正 XFF 信任問題**

現有設定使用 `$proxy_add_x_forwarded_for`，會把 client 自己送來的 `X-Forwarded-For` 保留在前面。而 `gin.Default()` 預設信任所有 proxy，取 client IP 時可能取到攻擊者偽造的值，使任何以 IP 為基礎的限流或日誌失去意義。

```go
r := gin.New()
r.Use(gin.Logger(), gin.Recovery())
r.SetTrustedProxies([]string{"172.16.0.0/12"}) // 依實際 docker network 調整
```

所有 location 的 `X-Forwarded-For` 也建議統一改成 `$remote_addr`。

**建議三（加分）**：除了 IP 限流，加上「每帳號連續失敗 N 次暫時鎖定」。純 IP 限流擋不了分散式猜密碼，也會誤傷共用 NAT 的校園網路。

### 8. 使用者管理：自刪、自降級、未驗證輸入

**位置**：`CreateUser`、`UpdateUser`、`DeleteUser`、`models.CreateUserRequest`

**問題**：
- 後端沒有阻止管理員刪除自己或把自己降為 staff。前端只用 `user.username !== 'admin'` 隱藏刪除鈕，繞過前端就能直接呼叫 API，最後一個 admin 可能被鎖死。
- `role` 沒有在應用層驗證白名單（可能只靠 DB CHECK，待確認）。
- 密碼沒有最小長度。`LoginRequest` 的 swagger example 是 `admin/admin`，建議移除。
- 修改角色或密碼後，既有 access token 仍有效至 15 分鐘到期，屬可接受取捨，但應寫進文件。

**建議**：

```go
type CreateUserRequest struct {
    Username string `json:"username" binding:"required,min=3,max=32"`
    Password string `json:"password" binding:"required,min=8,max=72"` // bcrypt 上限 72 bytes
    Name     string `json:"name"     binding:"required,max=64"`
    Role     string `json:"role"     binding:"required,oneof=admin staff"`
}

// DeleteUser / UpdateUser(Role) 內
selfID, _ := c.Get("userUUID")
if selfID == req.LID { c.JSON(403, gin.H{"error": "cannot modify your own account here"}); return }
```

另外，刪除或降級前要確認仍至少剩一位 admin。

### 9. Resolve 未檢查是否已解決

**位置**：`ResolveMaintenanceRecord`

**問題**：對已解決的紀錄再呼叫一次，會覆寫 `resolve_note`，並再次把設備的 `last_maint_date` 重設成今天，使保養週期被不當延後。

**建議**：

```sql
UPDATE maintenance_records
SET is_resolved = true, resolve_note = $1
WHERE lid = $2 AND is_resolved = false
RETURNING equipment_id
```

查無資料時再區分「不存在」與「已解決」，回 404 或 409。此外，`resolve_note` 也建議限制長度，並考慮記錄處理人（`resolved_by`、`resolved_at`）以便稽核。

---

## 🟡 中優先：工程品質

### 10. 新增／修改設備未處理重複 `asset_code`

`PostEquipment`、`UpdateEquipment` 遇到唯一鍵衝突（`23505`）時回 500，應回 409，與 `CreateUser` 的做法一致。另外 `MaintInterval` 沒有驗證下限（0 或負數會讓每次排程都判定到期），建議 `binding:"gte=1"`。`CreateEquipmentRequest` 的 `LastMaintDate` 是字串，格式錯誤時會變成 DB 錯誤而非 400，建議在應用層解析驗證。

### 11. 刪除設備會硬刪維修歷史

`DeleteEquipment` 先刪除所有 `maintenance_records` 再刪設備。對維修系統來說，歷史紀錄通常是稽核與統計（你的 Analytics 頁）的核心資料。建議改為 soft delete（`deleted_at` 或 `is_active`），查詢時過濾；或至少讓外鍵使用 `ON DELETE RESTRICT`，並在 README 說明取捨。

### 12. Cron 時區與 `CURRENT_DATE`

`cron.New()` 使用容器的本地時區，alpine 容器預設是 UTC，所以「每日 02:00」實際是台灣上午 10:00。`CURRENT_DATE` 則取決於 PostgreSQL 的時區設定。

```go
loc, _ := time.LoadLocation("Asia/Taipei")
c := cron.New(cron.WithLocation(loc))
```

DB 端可用 `CURRENT_DATE AT TIME ZONE` 的寫法，或在 compose 設定 `TZ`／`PGTZ`。若 Alpine 找不到時區資料，需在 final image 安裝 `tzdata`。啟動時 `go h.CheckAndCreateMaintenanceTasks()` 也應記錄錯誤並避免與排程同時執行。

### 13. 保養到期與故障狀態語意混淆

`CheckAndCreateMaintenanceTasks` 的程式碼註解本身就說明了問題：因為 CHECK 限制，把 `'system'` 改成 `'staff'`，並把「到期保養」的設備狀態設為 `faulty`。這會讓：
- 統計無法區分「真的壞了」與「該保養了」；
- 紀錄中的 `reporter_type` 不再代表真實來源。

前端 `types.ts` 其實已經有 `pending_maint`、`repairing` 狀態，表示設計上有預期過這件事。建議擴充 CHECK：

```sql
ALTER TABLE maintenance_records
  DROP CONSTRAINT IF EXISTS maintenance_records_reporter_type_check,
  ADD  CONSTRAINT maintenance_records_reporter_type_check
  CHECK (reporter_type IN ('public', 'staff', 'system'));

-- 並讓設備狀態包含 pending_maint
```

迴圈逐筆 INSERT/UPDATE 也可以改成兩條 set-based 語句（`INSERT ... SELECT`、`UPDATE ... FROM`），在設備量大時效率較好。

### 14. 錯誤處理與 API 回應不一致

- 部分 handler 用 `models.ErrorResponse`，多數用 `gin.H`，建議統一。
- `err == pgx.ErrNoRows` 應改 `errors.Is(err, pgx.ErrNoRows)`；`err.(*pgconn.PgError)` 改 `errors.As`，避免錯誤被包裝後失效。
- `CreateUser`、`UpdateUser` 等把 `err.Error()` 直接回給 client，會洩漏 validator 內部欄位名稱與格式，建議回固定訊息或只回欄位層級錯誤。
- `GetMaintenanceRecords` 忽略 `rows.Scan` 的錯誤，`GetDetailEquipment` 則是 `continue` 悄悄略過，資料會不完整卻回 200。
- `GetEquipment` 在 handler 內重複定義了與 `models.EquipmentPublicResponse` 相同的匿名 struct。
- 重複的 UUID 格式檢查（`22P02`）可集中成 helper，或在 binding 階段用 `uuid` 驗證。
- `resolved` query 參數目前只要不是 `"true"` 就視為 false，建議嚴格解析並回 400。

### 15. 分頁、測試、結構

- `GetUsers`、`GetDetailEquipment`、`GetMaintenanceRecords` 一次回傳全部資料，建議加 `limit`／`offset`（或 cursor），並設上限。
- 目前沒有任何測試。優先順序建議：`AuthMiddleware`／`RoleRequired`（用 `httptest`）、token 產生與驗證、保養排程的 SQL（可用 testcontainers 起 PostgreSQL）。
- `handlers.go` 單檔約 900 行，handler 同時負責驗證、SQL 與回應。建議拆成 `handlers/{auth,equipment,maintenance,user}.go`，進一步可抽出 repository 層，才方便測試。
- Swagger 的 `@host jupiterhsu.ddns.net` 寫死個人網域，建議移除或改為由環境變數決定。

### 16. `main.go`、Dockerfile、compose

**`main.go`**
- `r.Run()` 沒有 timeout，易受 Slowloris 攻擊。改用 `http.Server`，設定 `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout`，並處理 `SIGTERM` 做 graceful shutdown（同時停止 cron、關閉連線池）。
- 設定 `GIN_MODE=release`；Swagger 路由在正式環境建議關閉或加上驗證。
- 補 `/healthz`（可同時 ping DB），供 compose healthcheck 使用。

**`backend/Dockerfile`**
- `COPY . .` 在 `go mod download` 之前，任何原始碼變更都會讓依賴快取失效。先 `COPY go.mod go.sum`，下載依賴後再複製程式碼。
- 以 root 執行（`WORKDIR /root/`），改為建立非 root 使用者。
- 缺 `HEALTHCHECK`；建議編譯時加 `-ldflags="-s -w"` 縮小體積。
- 若要用 `Asia/Taipei`，記得在 final image 安裝 `tzdata`。

**`docker-compose.yaml`**
- `db` 沒有 healthcheck，`depends_on` 只保證啟動順序，不保證 DB 就緒（目前靠 `InitDB` 失敗後 `restart: always` 重試，能運作但不乾淨）。改用 `condition: service_healthy`。
- `./postgres_data` 是 bind mount，容易誤進版控並有權限問題，改成 named volume。
- `backend-net` 加上 `internal: true`，讓資料庫網路完全無法對外。
- 沒有 schema 初始化：`.gitignore` 的 `*.sql` 會忽略所有 SQL 檔。請移除該規則，並將 `db/init.sql` 掛到 `/docker-entrypoint-initdb.d/`。
- 補 `.env.example`；SSL 憑證路徑與網域改為可設定，並提供不需要憑證的 dev 設定。
- DB 連線字串用 `fmt.Sprintf` 直接拼接，密碼含 `@`、`/`、`:` 時會壞，改用 `url.URL{User: url.UserPassword(user, pass), ...}`；`sslmode=disable` 在內網可接受，但建議做成可設定。連線池也可明確設定 `MaxConns`、`MaxConnLifetime`。

**`frontend/Dockerfile`**
- Dockerfile 使用 bun 並 `COPY bun.lockb*`，但 repo 的鎖定檔是 `package-lock.json`。兩者不一致，`bun install --frozen-lockfile` 很可能失敗（待實測）。統一成一個套件管理器並提交對應的 lockfile。

### 17. Token 存放在 localStorage

`apiClient.ts`、`Login.tsx`、`AdminLayout.tsx` 把 access 與 refresh token 都存在 `localStorage`，一旦有 XSS，兩個 token 都會被讀走，而 refresh token 有效 1 天。

- 較好的做法：refresh token 改放 `HttpOnly; Secure; SameSite=Strict` cookie，access token 只放記憶體。
- 若短期不改，請在 README 的 Security 章節誠實列為「已知取捨」並加上 CSP 標頭（nginx `add_header Content-Security-Policy`）。
- `apiClient` 的 refresh 流程在多個請求同時 401 時會各自發出 refresh 請求，建議加一個共用的 refresh promise 做排隊。
- 登出應呼叫後端撤銷 refresh token（需要先完成第 5 點進階做法）。

---

## 🟡 前端

1. **沒有 route guard**：沒登入也能進入 `/admin` 的頁面外殼，只是 API 回 401 後才被導走。加一個 `<RequireAuth>` 包住 `/admin`，並依 role 隱藏 admin-only 的選單項目（`staff` 看得到「人員管理」連結但呼叫會被 403）。
2. **型別與後端不一致**：`types.ts` 使用 `id`，後端回傳 `lid`；`status` 包含 `pending_maint`、`repairing`，後端目前只出現 `normal`、`faulty`（待對照 DB CHECK）。前端多處以 `any` 繞過，等於型別檢查失效。建議由 swagger 產生型別（如 `openapi-typescript`），或至少手動對齊。
3. **`any` 與錯誤處理**：`useState<any[]>`、`catch (err: any)`、`const payload: any` 改為具體型別與 `unknown` 搭配 type guard。
4. **使用者體驗**：大量使用 `alert()`，建議改為 toast 或行內錯誤訊息；`ReportEquipment` 對所有錯誤都顯示「可能已有通報紀錄」，應區分 409、429、網路錯誤。
5. **殘留**：`console.log`（登入、登出、報修）、空的 `mock/data.ts`、`frontend/README.md`（Vite 範本）、`build/index.html`（「前端測試中」）、`dev-dist/`（建置產物）。`ReportEquipment` 的 `asset_code` 直接字串串接進 URL，應使用 `encodeURIComponent` 或 axios `params`。
6. **PWA**：`vite.config.ts` 的 `devOptions.enabled: true` 會在開發環境啟用 Service Worker，常造成快取干擾；manifest 參照的 `pwa-192x192.png`、`pwa-512x512.png` 在 `public/` 中看不到，需補上，否則安裝時會失敗。

---

## 建議執行順序

1. **第 1、2 點**一起修（JWT secret），並補 `.env.example`。
2. **第 3、4、9 點**：`reporter_type` 與長度限制、partial unique index、resolve 條件。
3. **第 7 點**：nginx 加登入限流，順便修 XFF。
4. **第 5、6、8 點**：refresh 驗證、timing、使用者管理保護。
5. 補 `db/init.sql` 與 compose 修正（第 16 點），確認 `docker compose up` 能完整跑起來。
6. 其餘依時間調整；若要宣稱「可撤銷 token」，才做第 5 點進階版。

> 提醒：README 的 Security 章節只寫已實作的項目，並另列「已知限制與 Roadmap」。能清楚說明自己的取捨，比宣稱所有項目都完美更有說服力。
