# 修改任務單 #3：資安與部署必修（共 7 項）

> 審查基準：HEAD `593c523`。前兩份任務單的其餘項目已驗證完成，本文件只列剩餘必修。
> 編號：`T1`–`T2` 資安、`T3`–`T7` 部署（含 podman）。

## 0. 先讀這裡

**不得變更的測試帳號（僅 dev，`db/seed_dev.sql`）**

| 帳號 | 密碼 | 角色 |
|---|---|---|
| `admin` | `admin123456` | admin |
| `staff01` | `admin123456` | staff |

**範圍限制**：只改本文件列出的項目。不要變動其他行為、不要改上述帳號、不要移除 dev seed。

**驗證標示**：標「實測」的是審查者已實際重現；標「依已知行為」的是審查者無法在沙箱測試，請你在 PR 說明誠實標註「已驗證／未驗證」，不得宣稱沒跑過的東西已通過。

---

## 資安

### T1. DB 密碼護欄沒有擋 `change_this`

- **問題（實測）**：README L103 與 `.env.example` L1 都說 production 下 `DB_PASSWORD` 含 `change_this` 會拒絕啟動，但 `backend/internal/database/db.go` 只檢查空值、`postgres`、長度 < 8。用 `change_this_to_a_secure_password`（即 `.env.example` 預設值）在 `APP_ENV=production` 下啟動，後端正常運作。
- **做法**：
  1. 把檢查抽成可測試的 `validateDBPassword(pass string) error`：空值、`postgres`、長度 < 8、**不分大小寫含 `change_this`** 皆回傳錯誤；`InitDB` 在 production 下呼叫它並 `os.Exit(1)`。
  2. 補單元測試（表格式）：`""`、`postgres`、`short1`、`change_this_to_a_secure_password`、`CHANGE_THIS_x12345`、`Str0ngDbPass99`。
  3. README 與 `.env.example` 的文字需與實作一致。
- **驗收**：上述測試通過；`APP_ENV=production` 搭配 `DB_PASSWORD=change_this_to_a_secure_password` 啟動後端 → 以護欄訊息退出；dev 不受影響。

### T2. `APP_ENV` 判斷不一致，`prod` 會繞過 DB 護欄

- **問題（實測）**：`middleware.InitJWT` 接受 `production` 與 `prod`（不分大小寫）；`database.InitDB` 只認小寫 `production`。`APP_ENV=prod` 時 DB 護欄不生效。
- **做法**：新增共用函式（建議 `backend/internal/config/env.go` 的 `IsProduction() bool`）：`strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))` 為 `production` 或 `prod` 時回傳 true；`InitJWT`、`InitDB` 都改用它（注意避免 import 循環）。
- **驗收**：單元測試 `production`、`Production`、` PRODUCTION `、`prod` → true；空字串、`development`、`staging` → false。`APP_ENV=prod` 搭配 `DB_PASSWORD=postgres` → 被擋。

---

## 部署（podman）

### T3. README 升級步驟 3 的 `pg_restore` 無法照做

- **位置**：README「版本升級指南」步驟 3（約 L243–248）。
- **問題（實測）**：
  1. 指令沒有用 `sh -c '…'` 包起來（步驟 4 有），`$POSTGRES_USER`、`$POSTGRES_DB` 由**宿主 shell** 展開，沒 export 時為空。
  2. 即使修正，`--clean` 還原到已由 `init.sql` 建好的庫，會因 `refresh_tokens` 的外鍵擋住 `users` 的 DROP，產生 4 個錯誤、exit 1（資料其實都還原了）。
- **做法**：步驟 3 改為兩個指令：
  ```bash
  # 3-1 移除升級時本來就是空的 refresh_tokens（避免外鍵擋住 users 的 DROP）
  podman compose exec -T db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "DROP TABLE IF EXISTS refresh_tokens"'

  # 3-2 還原
  podman compose exec -T db sh -c 'pg_restore --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < backup_beta_1_2.dump
  ```
  並以一句話說明 3-1 的原因；步驟 4 的 001、002 維持不變（`refresh_tokens` 由 001 重建）。
- **驗收**：審查者已用純 PostgreSQL（`pg_dump -Fc` → 新庫 `init.sql` → 先 DROP → `pg_restore --clean --if-exists --no-owner`）實測：0 錯誤、exit 0，之後 001＋002 成功且筆數一致。你若有 podman 環境，請在拋棄式環境以相同流程演練並貼出輸出；否則標註「未以容器演練」。

### T4. prod 的 `!override` 需要夠新的 compose provider

- **位置**：README L26、L79。
- **問題（實測）**：`docker-compose.prod.yaml` 使用 `!override`，需 docker-compose ≥ 2.24.4 **或** podman-compose ≥ **1.4.0**。podman-compose 1.0.6、1.1.0、1.2.0、1.3.0 解析 prod 設定會失敗：`could not determine a constructor for the tag '!override'`（Ubuntu 24.04 的 apt 版是 1.0.6）。dev 與 test 設定在 1.0.6 可正常解析，只有 prod 受影響。`podman compose` 在兩種 provider 都安裝時優先使用 docker-compose。README L79 的路徑 `/usr/lib/docker/cli-plugins/docker-compose` 在審查者測的 Ubuntu 24.04 實際是 `/usr/libexec/docker/cli-plugins/docker-compose`，路徑依發行版而異。
- **做法（最小）**：README 改寫版本需求，並加入：
  - 最低版本：docker-compose ≥ 2.24.4，或 podman-compose ≥ 1.4.0。
  - 檢查方式：`podman compose version`（會顯示實際使用的 provider 與版本）。
  - 症狀：若看到 `could not determine a constructor for the tag '!override'`，代表 provider 太舊，請升級（例如 `pip install --user -U podman-compose`）。
  - 移除寫死的 provider 路徑，改為「路徑依發行版而異」。
- **（選做）** 重構成不依賴 `!override`：基底 `docker-compose.yaml` 不放 dev 專屬項目，dev 專屬設定放 `docker-compose.override.yaml`（單獨執行 compose 時自動載入），prod 以 `-f docker-compose.yaml -f docker-compose.prod.yaml` 明確指定。若採用，需確認 dev 的 `podman compose up -d` 行為不變，並以 docker-compose 與 podman-compose 1.0.6 各跑一次 `config`。
- **驗收**：README 無矛盾敘述；若做了選做項，`config` 輸出與現行一致（prod：ports 僅 80／443、掛載僅 `nginx.prod.conf` 與 `ssl`、db 無 seed）。

### T5. rootless podman 預設無法綁 80／443（依已知行為）

- **問題**：prod override 綁 `80:80`、`443:443`；rootless podman 預設 `net.ipv4.ip_unprivileged_port_start=1024`，會以 `rootlessport cannot expose privileged port 80` 之類的錯誤失敗。README 沒提。
- **做法**：在 README「生產環境部署指南」加一個小節（rootless podman 注意事項），提供二選一：
  ```bash
  # 做法 A：允許非特權使用者綁 80 以上的埠，並設為開機生效
  echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/99-unprivileged-ports.conf
  sudo sysctl --system

  # 做法 B：改用 rootful podman
  sudo podman compose -f docker-compose.yaml -f docker-compose.prod.yaml --env-file .env up -d
  ```
  （若採 B，需註明 `.env`、`ssl/` 路徑與權限要對 root 可讀。）
- **驗收**：README 有此小節，且措辭標明「rootless podman 預設」。

### T6. SELinux 主機的 bind mount（依已知行為）

- **問題**：所有 bind mount 都沒有 SELinux 標籤。在 SELinux enforcing 的主機（Fedora、RHEL、Rocky 等）上，容器讀取 `nginx.*.conf`、`ssl/`、`init.sql`、`seed_dev.sql` 會 permission denied。Ubuntu／Debian 不受影響。
- **做法**：
  1. 把下列 bind mount 的 `:ro` 改為 `:ro,Z`（docker 在非 SELinux 環境會忽略，podman 與 docker 皆可用）：
     - `docker-compose.yaml`：`nginx.dev.conf`、`init.sql`、`seed_dev.sql`
     - `docker-compose.prod.yaml`：`nginx.prod.conf`、`ssl`、`init.sql`
     - `docker-compose.test.yaml`：`init.sql`
  2. README 加註：`getenforce` 顯示 `Enforcing` 時才需要；`:Z` 會重新標記宿主檔案，**不要**對家目錄等大範圍目錄使用。
- **驗收**：docker-compose 2.24.4+ 與 podman-compose ≥ 1.4.0 的 `config` 都能解析，且各掛載的 source／target 不變；SELinux 實際行為你若無環境可測，PR 說明標註「未驗證」。

### T7. test compose 的映像是短名稱

- **位置**：`docker-compose.test.yaml` L3 `image: postgres:15-alpine`。
- **問題**：其他 compose 與 Dockerfile 都用 `docker.io/library/...`。podman 在未設定 `unqualified-search-registries` 或 short-name 為 enforcing 時會提示或失敗；README 的整合測試指令正是用 podman 跑這個檔。
- **做法**：改為 `image: docker.io/library/postgres:15-alpine`。
- **驗收**：`config` 解析正常；README 的整合測試流程可照做。

---

## 完成定義

```bash
# 後端（Go 1.25）
cd backend && gofmt -l . && go build ./... && go vet ./... && go test ./... -count=1

# 後端整合測試（隔離測試庫）
podman compose -f docker-compose.test.yaml up -d db-test
cd backend && DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=postgres DB_PASSWORD=postgres \
  DB_NAME=gets_test JWT_SECRET=integration_test_jwt_secret_32_chars_minimum_value \
  REQUIRE_DB_TESTS=1 go test ./... -v -count=1
podman compose -f docker-compose.test.yaml down -v

# compose 解析（至少 docker-compose；若有 podman-compose ≥ 1.4.0 也請跑）
docker compose config -q
JWT_SECRET=$(openssl rand -hex 32) DB_PASSWORD=Str0ngDbPass99 \
  docker compose -f docker-compose.yaml -f docker-compose.prod.yaml config -q

# dev 冒煙（確認測試帳號未被改動）
bash scripts/smoke_dev.sh
```

PR 說明必須列出：哪些已實際執行、哪些未能執行（例如沒有 SELinux 主機、沒有 rootless podman、沒有線上資料庫）。
