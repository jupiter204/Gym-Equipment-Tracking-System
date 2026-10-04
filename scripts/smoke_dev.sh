#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:${HTTP_PORT:-8000}"
FAILED=0

echo "=== GETS Dev Smoke Test ($BASE_URL) ==="

check_case() {
    local desc="$1"
    local result="$2"
    if [ "$result" -eq 0 ]; then
        echo -e "[\033[32mPASS\033[0m] $desc"
    else
        echo -e "[\033[31mFAIL\033[0m] $desc"
        FAILED=1
    fi
}

# 1. POST /api/auth/login admin / admin123456 -> 200, contains access_token & refresh_token
ADMIN_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123456"}')
ADMIN_CODE=$(echo "$ADMIN_RESP" | tail -n 1)
ADMIN_BODY=$(echo "$ADMIN_RESP" | head -n -1)
if [ "$ADMIN_CODE" = "200" ] && echo "$ADMIN_BODY" | grep -q "access_token" && echo "$ADMIN_BODY" | grep -q "refresh_token"; then
    check_case "POST /api/auth/login (admin / admin123456) -> 200 with tokens" 0
    ADMIN_TOKEN=$(echo "$ADMIN_BODY" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
else
    check_case "POST /api/auth/login (admin / admin123456) -> 200 with tokens" 1
    ADMIN_TOKEN=""
fi

# 2. POST /api/auth/login staff01 / admin123456 -> 200
STAFF_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"staff01","password":"admin123456"}')
STAFF_CODE=$(echo "$STAFF_RESP" | tail -n 1)
STAFF_BODY=$(echo "$STAFF_RESP" | head -n -1)
if [ "$STAFF_CODE" = "200" ] && echo "$STAFF_BODY" | grep -q "access_token"; then
    check_case "POST /api/auth/login (staff01 / admin123456) -> 200" 0
    STAFF_TOKEN=$(echo "$STAFF_BODY" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
else
    check_case "POST /api/auth/login (staff01 / admin123456) -> 200" 1
    STAFF_TOKEN=""
fi

# 3. POST /api/auth/login admin / admin123 -> 401
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}')
if [ "$CODE" = "401" ]; then
    check_case "POST /api/auth/login (admin / admin123) -> 401" 0
else
    check_case "POST /api/auth/login (admin / admin123) -> 401 (got $CODE)" 1
fi

# 4. POST /api/auth/login staff01 / staff123 -> 401
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"staff01","password":"staff123"}')
if [ "$CODE" = "401" ]; then
    check_case "POST /api/auth/login (staff01 / staff123) -> 401" 0
else
    check_case "POST /api/auth/login (staff01 / staff123) -> 401 (got $CODE)" 1
fi

# 5. admin token -> GET /api/private/stats -> 200
if [ -n "$ADMIN_TOKEN" ]; then
    CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$BASE_URL/api/private/stats" \
        -H "Authorization: Bearer $ADMIN_TOKEN")
    if [ "$CODE" = "200" ]; then
        check_case "Admin token -> GET /api/private/stats -> 200" 0
    else
        check_case "Admin token -> GET /api/private/stats -> 200 (got $CODE)" 1
    fi
else
    check_case "Admin token -> GET /api/private/stats -> 200 (Skipped: no admin token)" 1
fi

# 6. staff token -> GET /api/private/users -> 403
if [ -n "$STAFF_TOKEN" ]; then
    CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$BASE_URL/api/private/users" \
        -H "Authorization: Bearer $STAFF_TOKEN")
    if [ "$CODE" = "403" ]; then
        check_case "Staff token -> GET /api/private/users -> 403" 0
    else
        check_case "Staff token -> GET /api/private/users -> 403 (got $CODE)" 1
    fi
else
    check_case "Staff token -> GET /api/private/users -> 403 (Skipped: no staff token)" 1
fi

# 7. No token -> GET /api/private/equipments -> 401
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$BASE_URL/api/private/equipments")
if [ "$CODE" = "401" ]; then
    check_case "No token -> GET /api/private/equipments -> 401" 0
else
    check_case "No token -> GET /api/private/equipments -> 401 (got $CODE)" 1
fi

# 8. GET /api/public/equipment?asset_code=RUN-001 -> 200
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$BASE_URL/api/public/equipment?asset_code=RUN-001")
if [ "$CODE" = "200" ]; then
    check_case "GET /api/public/equipment?asset_code=RUN-001 -> 200" 0
else
    check_case "GET /api/public/equipment?asset_code=RUN-001 -> 200 (got $CODE)" 1
fi

# 9. GET /healthz -> 200
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$BASE_URL/healthz")
if [ "$CODE" = "200" ]; then
    check_case "GET /healthz -> 200" 0
else
    check_case "GET /healthz -> 200 (got $CODE)" 1
fi

echo "========================================"
if [ "$FAILED" -eq 0 ]; then
    echo -e "\033[32mAll smoke tests PASSED!\033[0m"
    exit 0
else
    echo -e "\033[31mSmoke test FAILED!\033[0m"
    exit 1
fi
