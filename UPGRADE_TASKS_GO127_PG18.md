# 升級任務單：Go 1.27.1 與 PostgreSQL 18.6

> 目標：後端改用 **Go 1.27.1**，資料庫改用 **PostgreSQL 18.6**（皆為目前最新穩定版本）。
> 對象：負責修改原始碼的 agent。基準：目前專案為 Go 1.25.0（`go.mod`）／builder `golang:1.26.2-alpine3.22`／`postgres:15-alpine`。

## 0. 版本確認結果（查證日期：2026-10-06）

| 項目 | 確認結果 | 依據 |
| :--- | :--- | :--- |
| Go 1.27.1 | **存在，且為 1.27 系列最新** | GitHub `golang/go` 有 `go1.27.1` tag（1.27.0、1.27.1 皆已發布）；docker-library 的 `golang` 映像 metadata 顯示 1.27 系列版本為 `1.27.1`，變體含 `alpine3.24`、`alpine3.23` |
| PostgreSQL 18.6 | **存在，且為 18 系列最新** | GitHub `postgres/postgres` 有 `REL_18_6` tag；docker-library 的 `postgres` 映像 metadata 顯示 18 系列版本為 `18.6`，變體含 `alpine3.24`、`alpine3.23`；其 Dockerfile 內 `PG_VERSION 18.6` |
| 18 是否為最新穩定主版本 | 是 | 19 目前仍是 beta（`19beta4`、`REL_19_BETA4`），**不要升級到 19** |

**查證限制**：以上是從 GitHub tag 與 docker-library 的官方 metadata／Dockerfile 確認，審查者**沒有**從映像倉庫實際拉取映像。請你先執行下列指令確認標籤可用，任何一個失敗都先回報，不要自行改用其他標籤：

```bash
podman pull docker.io/library/golang:1.27.1-alpine3.24
podman pull docker.io/library/postgres:18.6-alpine3.24
podman pull docker.io/library/alpine:3.24
```

合併前請再確認一次是否有更新的修補版本（例如 1.27.2、18.7）；若有，改用新版並在 PR 說明。

## 1. 範圍與限制

- 只做本文件列出的升級，不改應用程式行為、不改資料表結構。
- **不得更動開發測試帳號**：`admin` / `admin123456`、`staff01` / `admin123456`（`db/seed_dev.sql`）。
- **不要**升級到 PostgreSQL 19 beta；**不要**對 Go 依賴做全面升級（不要 `go get -u ./...`）。
- 前端與 Node 版本不在範圍內。

## 2. 重要風險（先讀）

**PostgreSQL 18 的官方映像改了資料目錄。** 依官方 Dockerfile：18 的 `PGDATA` 是 `/var/lib/postgresql/18/docker`、`VOLUME` 是 `/var/lib/postgresql`；15 則是 `/var/lib/postgresql/data`。專案目前三個 compose 檔都掛在舊路徑，**只改映像標籤、不改掛載路徑會讓資料無法正確持久化**。

**15 → 18 是主版本升級**：資料目錄格式不相容，既有部署必須用 `pg_dump`／`pg_restore` 搬遷，不能直接換映像。若沿用舊卷名稱與新映像，PostgreSQL 18 會在卷內另建空的 `18/docker`，舊資料原封不動留在 `data/`，看起來像資料全部消失。

## 3. 任務

### U1. Go 1.27.1

- **位置**：`backend/go.mod` L3（`go 1.25.0`）、`backend/Dockerfile` L2（builder）與 L14（runtime）。
- **做法**：
  1. `go.mod` 的 `go` 指令改為 `go 1.27.1`。
  2. `backend/Dockerfile`：builder 改為 `docker.io/library/golang:1.27.1-alpine3.24`；runtime 改為 `docker.io/library/alpine:3.24`（與 builder 的 Alpine 版本一致；原本的 `alpine:3.22.4` 已不在 golang 映像支援的變體內）。若要固定修補版號，必須先確認該標籤存在。
  3. 閱讀 Go 1.27 官方 release notes（https://go.dev/doc/go1.27），重點看語言、工具鏈、`go vet`、runtime 與 GODEBUG 的變更；在 PR 說明列出「與本專案相關的項目」，沒有也要寫「已檢視，無相關項目」。
  4. 執行 `go mod tidy`。若建置或測試因舊依賴失敗，**只升級必要的模組**，並在 PR 說明原因。
  5. 確認 `gofmt -l .`、`go build ./...`、`go vet ./...` 全部乾淨。
- **驗收**：
  - `podman run --rm docker.io/library/golang:1.27.1-alpine3.24 go version` 輸出 `go1.27.1`。
  - 後端映像可建置、容器可啟動、`/healthz` 回 200。
  - `go.mod`、`Dockerfile` 內不再有 1.25／1.26／alpine3.22 的殘留（`grep -rn -E '1\.25|1\.26|alpine3\.22' backend/go.mod backend/Dockerfile` 無結果）。

### U2. PostgreSQL 18.6 映像與資料卷

- **位置與變更**：

  | 檔案 | 行 | 現況 | 改為 |
  | :--- | :--- | :--- | :--- |
  | `docker-compose.yaml` | 38 | `postgres:15-alpine` | `docker.io/library/postgres:18.6-alpine3.24` |
  | `docker-compose.yaml` | 46 | `postgres_data:/var/lib/postgresql/data` | `postgres18_data:/var/lib/postgresql` |
  | `docker-compose.yaml` | 58–59 | 宣告 `postgres_data:` | 宣告 `postgres18_data:` |
  | `docker-compose.prod.yaml` | 24 | `postgres_data:/var/lib/postgresql/data` | `postgres18_data:/var/lib/postgresql` |
  | `docker-compose.test.yaml` | 3 | `postgres:15-alpine` | `docker.io/library/postgres:18.6-alpine3.24` |
  | `docker-compose.test.yaml` | 11 | tmpfs `/var/lib/postgresql/data` | tmpfs `/var/lib/postgresql` |

- **為什麼換新卷名稱**：避免新映像掛到舊卷而悄悄建出空資料庫，也讓舊資料保留在原卷，作為回滾保險。
- **注意**：
  - `docker-compose.prod.yaml` 的 `db.volumes` 使用 `!override`，改掛載路徑時要連同 `init.sql` 的 `:ro,Z` 掛載一起保留。
  - 精確標籤 `18.6-alpine3.24` 確保可重現；日後 18.x 的**小版本**升級只需換標籤，不需要 dump／restore。
  - `init.sql`、`seed_dev.sql`、`migrations/001`、`002` 是在 PostgreSQL 16 上驗證過的，**必須在 18.6 上重新完整驗證**（見 U5）。
- **驗收**：
  - dev、prod、test 三種 `config` 都能解析，且 db 的掛載目標為 `/var/lib/postgresql`。
  - `podman compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc 'select version()'` 顯示 `PostgreSQL 18.6`。
  - **持久化測試（必做）**：建立一筆資料 → `podman compose down`（**不加 `-v`**）→ `up -d` → 資料仍在。

### U3. 既有環境的資料搬遷（15 → 18）

在 README 新增「PostgreSQL 15 → 18 升級」一節，涵蓋以下三種情境，每個指令都要能直接複製執行（用 `sh -c '…'` 讓環境變數在容器內展開，與現有升級指南一致）：

**A. 開發環境**：沒有要保留的資料時，`podman compose down -v` 後重新 `up -d`（結構與測試資料會重新載入）。說明舊卷 `postgres_data` 會成為孤兒，確認不需要後再手動移除。

**B. 正式環境（目前版本、PostgreSQL 15）**：

```bash
# 1. 在「舊 stack（仍是 PostgreSQL 15）」上備份
podman compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > backup_pg15.dump

# 2. 停止舊 stack（不要加 -v，舊卷保留作為回滾保險）
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml down

# 3. 切換到新版程式碼後，只啟動新的資料庫並等待 healthy
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml up -d db

# 4. 還原
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml exec -T db sh -c 'pg_restore --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < backup_pg15.dump

# 5. 啟動其餘服務並驗證
podman compose -f docker-compose.yaml -f docker-compose.prod.yaml up -d
```

- 這種來源的備份已包含 `refresh_tokens`，**不需要**現有指南的「先 DROP `refresh_tokens`」步驟，也不需要再跑 001／002；請在演練時確認 `pg_restore` 為 0 錯誤、exit 0。
- 回滾：保留舊卷與備份，驗證完成前不要刪除；回滾時切回舊版程式碼與舊卷。

**C. 正式環境（仍是 beta-1.2）**：沿用現有「從 Beta-1.2 升級」指南（含步驟 3-1 與 001、002），目標資料庫為 PostgreSQL 18。在該指南開頭加註：新版 compose 的資料卷已是 `postgres18_data`、掛載點為 `/var/lib/postgresql`。

- **驗收**：在拋棄式環境以 **PostgreSQL 15 的備份**實際演練情境 B 與 C，貼出 `pg_restore` 的 exit code、錯誤數，以及還原前後的資料筆數（使用者、設備、維修紀錄、未解決紀錄）。沒有容器環境無法演練時，PR 說明明確標註「未演練」。

### U4. 文件更新

README（請以關鍵字搜尋，行號依版本而異）：

- 技術棧／系統架構：Go 版本改為 **1.27+**；PostgreSQL 改為 **18**（映像 `postgres:18.6-alpine3.24`），刪除「15／16」「16 亦通過測試」之類舊描述。
- 所有提到具名卷 `postgres_data` 或掛載點 `/var/lib/postgresql/data` 之處，改為 `postgres18_data` 與 `/var/lib/postgresql`。
- 新增 U3 的升級章節，並在版本升級指南中說明兩者的關係。
- 在 README 的適當位置註明「已驗證版本：Go 1.27.1、PostgreSQL 18.6」。
- 若 README 提到 `down -v` 的行為，確認說明仍正確（卷名稱已改）。

### U5. 驗證

全部通過後才可合併，並在 PR 說明誠實列出已執行與未能執行的項目：

```bash
# 後端（Go 1.27.1）
cd backend && gofmt -l . && go build ./... && go vet ./... && go test ./... -count=1

# 整合測試（隔離測試庫，PostgreSQL 18.6）
podman compose -f docker-compose.test.yaml up -d db-test
cd backend && DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=postgres DB_PASSWORD=postgres \
  DB_NAME=gets_test JWT_SECRET=integration_test_jwt_secret_32_chars_minimum_value \
  REQUIRE_DB_TESTS=1 go test ./... -v -count=1
podman compose -f docker-compose.test.yaml down -v

# compose 解析（dev／prod／test）
podman compose config -q
JWT_SECRET=$(openssl rand -hex 32) DB_PASSWORD=Str0ngDbPass99 \
  podman compose -f docker-compose.yaml -f docker-compose.prod.yaml config -q
podman compose -f docker-compose.test.yaml config -q

# dev 冒煙與持久化
podman compose up -d && bash scripts/smoke_dev.sh
# 持久化：寫入資料 → podman compose down（不加 -v）→ up -d → 資料仍在
```

額外確認：
- 在 **18.6** 上，`init.sql`＋`seed_dev.sql` 可重複執行、`migrations/001`＋`002` 在模擬的舊 schema 上可重複執行（沿用 README 升級指南的演練方式）。
- 整合測試共 24 個測試全部通過，沒有因版本差異而被略過。

## 4. 完成定義

1. U1–U5 全部完成並通過驗收。
2. 專案內不再有 `go 1.25`、`golang:1.26`、`alpine3.22`、`postgres:15`、`/var/lib/postgresql/data` 的殘留（`grep -rn` 確認，README 的歷史說明除外）。
3. PR 說明包含：Go 1.27 release notes 的相關項目、`go mod tidy` 後若有升級的模組與原因、U3 的演練結果（或「未演練」）、無法驗證的項目。**不得宣稱沒跑過的東西已通過。**
