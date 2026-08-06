#!/bin/bash
# MFDS 1차 정제 검증 하네스 — 실데이터 스냅샷 기반 검증 도구
# 읽기 전용: declarations 테이블을 수정하지 않음
# 규칙문서 10장 지표: RCNO 수 불변, SKU 후보 수 변화, review_required 비율, 결함 1~4 건수
set -euo pipefail

MYSQL="docker exec mfds-mysql mysql --default-character-set=utf8mb4 -umfds -pmfds mfds_ledger"

echo "============================================"
echo " MFDS 1차 정제 검증 리포트"
echo " 규칙문서 10장 기반"
echo " 생성시각: $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================"
echo ""

# --- 1. 기본 현황 ---
echo "--- 1. 기본 현황 ---"
echo ""

TOTAL_DECLARATIONS=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations;")
echo "  declarations 행 수          : $TOTAL_DECLARATIONS"

TOTAL_ITEMS=$($MYSQL -N -e "SELECT COUNT(*) FROM items;")
echo "  items 행 수                 : $TOTAL_ITEMS"

DISTINCT_RCNO=$($MYSQL -N -e "SELECT COUNT(DISTINCT rcno) FROM declarations;")
echo "  고유 RCNO 수                : $DISTINCT_RCNO"

echo ""

# --- 2. 정제 상태 분포 ---
echo "--- 2. 정제 상태 분포 ---"
echo ""

$MYSQL -t -e "
SELECT normalization_status AS status, COUNT(*) AS count,
       ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM declarations), 2) AS pct
FROM declarations
GROUP BY normalization_status
ORDER BY count DESC;
"

echo ""

# --- 3. SKU 후보 현황 ---
echo "--- 3. SKU 후보 현황 ---"
echo ""

SKU_CANDIDATE_COUNT=$($MYSQL -N -e "
SELECT COUNT(DISTINCT sku_candidate_key_sha256) FROM declarations
WHERE sku_candidate_key_sha256 IS NOT NULL;
")
echo "  SKU 후보 키 수 (DISTINCT)   : $SKU_CANDIDATE_COUNT"

SKU_CANDIDATE_ROWS=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations WHERE sku_candidate_key_sha256 IS NOT NULL;
")
echo "  SKU 후보 키 보유 행 수      : $SKU_CANDIDATE_ROWS"

echo ""

# --- 4. review_required 비율 ---
echo "--- 4. review_required 비율 ---"
echo ""

REVIEW_REQUIRED=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations WHERE normalization_status = 'REVIEW_REQUIRED';
")
REVIEW_PCT=$(echo "scale=2; 100.0 * $REVIEW_REQUIRED / $TOTAL_DECLARATIONS" | bc)
echo "  REVIEW_REQUIRED              : $REVIEW_REQUIRED ($REVIEW_PCT%)"

PARTIAL=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE normalization_status = 'PARTIAL';")
echo "  PARTIAL                      : $PARTIAL"

NORMALIZED=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE normalization_status = 'NORMALIZED';")
echo "  NORMALIZED                   : $NORMALIZED"

UNPARSED=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE normalization_status = 'UNPARSED';")
echo "  UNPARSED                     : $UNPARSED"
echo ""

# --- 5. 용량 / ABV / 숙성 / vintage 추출률 ---
echo "--- 5. 추출률 ---"
echo ""

VOLUME_EXTRACTED=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE unit_volume_ml IS NOT NULL;")
VOLUME_PCT=$(echo "scale=1; 100.0 * $VOLUME_EXTRACTED / $TOTAL_DECLARATIONS" | bc)
echo "  용량 추출                    : $VOLUME_EXTRACTED ($VOLUME_PCT%)"

ABV_EXTRACTED=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE abv_percent IS NOT NULL;")
ABV_PCT=$(echo "scale=1; 100.0 * $ABV_EXTRACTED / $TOTAL_DECLARATIONS" | bc)
echo "  ABV 추출                     : $ABV_EXTRACTED ($ABV_PCT%)"

AGE_EXTRACTED=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE age_years IS NOT NULL;")
AGE_PCT=$(echo "scale=1; 100.0 * $AGE_EXTRACTED / $TOTAL_DECLARATIONS" | bc)
echo "  숙성연수 추출                : $AGE_EXTRACTED ($AGE_PCT%)"

VINTAGE_EXTRACTED=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations WHERE vintage_year IS NOT NULL;")
VINTAGE_PCT=$(echo "scale=1; 100.0 * $VINTAGE_EXTRACTED / $TOTAL_DECLARATIONS" | bc)
echo "  vintage 추출                 : $VINTAGE_EXTRACTED ($VINTAGE_PCT%)"

BASE_UNCHANGED=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations d
JOIN items i ON i.id = d.source_item_id
WHERE d.base_product_name_ko = i.product_name_ko AND d.base_product_name_en = i.product_name_en
  AND d.sku_display_name_ko = i.product_name_ko AND d.sku_display_name_en = i.product_name_en;
")
BASE_PCT=$(echo "scale=1; 100.0 * $BASE_UNCHANGED / $TOTAL_DECLARATIONS" | bc)
echo "  base가 원본과 무변화         : $BASE_UNCHANGED ($BASE_PCT%)"
echo ""

# --- 6. 결함 1: 괄호 짝 불일치 ---
echo "--- 6. 결함 1: 괄호 짝 불일치 ---"
echo ""

KO_MISMATCH=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE base_product_name_ko IS NOT NULL
  AND (CHAR_LENGTH(base_product_name_ko) - CHAR_LENGTH(REPLACE(base_product_name_ko, '(', '')))
    != (CHAR_LENGTH(base_product_name_ko) - CHAR_LENGTH(REPLACE(base_product_name_ko, ')', '')));
")
echo "  KO base_product_name 괄호 불일치: $KO_MISMATCH"

EN_MISMATCH=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE base_product_name_en IS NOT NULL
  AND (CHAR_LENGTH(base_product_name_en) - CHAR_LENGTH(REPLACE(base_product_name_en, '(', '')))
    != (CHAR_LENGTH(base_product_name_en) - CHAR_LENGTH(REPLACE(base_product_name_en, ')', '')));
")
echo "  EN base_product_name 괄호 불일치: $EN_MISMATCH"

KO_SKU_MISMATCH=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE sku_display_name_ko IS NOT NULL
  AND (CHAR_LENGTH(sku_display_name_ko) - CHAR_LENGTH(REPLACE(sku_display_name_ko, '(', '')))
    != (CHAR_LENGTH(sku_display_name_ko) - CHAR_LENGTH(REPLACE(sku_display_name_ko, ')', '')));
")
echo "  KO sku_display_name 괄호 불일치 : $KO_SKU_MISMATCH"
echo ""

# --- 7. 결함 2: 빈 괄호 잔존 ---
echo "--- 7. 결함 2: 빈 괄호 잔존 ---"
echo ""

EMPTY_PARENS_KO=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE base_product_name_ko LIKE '%()%' OR base_product_name_ko LIKE '%[]%';
")
echo "  KO base_name 빈 괄호 잔존   : $EMPTY_PARENS_KO"

EMPTY_PARENS_EN=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE base_product_name_en LIKE '%()%' OR base_product_name_en LIKE '%[]%';
")
echo "  EN base_name 빈 괄호 잔존   : $EMPTY_PARENS_EN"

EMPTY_PARENS_SKU=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE sku_display_name_ko LIKE '%()%' OR sku_display_name_ko LIKE '%[]%';
")
echo "  KO sku_display 빈 괄호 잔존 : $EMPTY_PARENS_SKU"

# 다중 괄호가 있는 원본 대비 결과 추정
MULTI_PAREN=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations d JOIN items i ON i.id = d.source_item_id WHERE (i.product_name_ko REGEXP '\\\\([^)]*\\\\(' OR i.product_name_ko REGEXP '\\\\)[^(]*\\\\)') OR (i.product_name_en REGEXP '\\\\([^)]*\\\\(' OR i.product_name_en REGEXP '\\\\)[^(]*\\\\)')")
echo "  원본 다중 괄호 (2개 이상)   : $MULTI_PAREN"
echo ""

# --- 8. 결함 3: 쉼표/슬래시 잔재 ---
echo "--- 8. 결함 3: 쉼표/슬래시 잔재 ---"
echo ""

COMMA_REMNANTS=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE base_product_name_ko LIKE '%,,%'
   OR base_product_name_ko LIKE '%, ,%'
   OR base_product_name_ko LIKE '% , %'
   OR sku_display_name_ko LIKE '%,,%'
   OR sku_display_name_ko LIKE '%, ,%'
   OR sku_display_name_ko LIKE '% , %';
")
echo "  쉼표 잔재 (연속쉼표/빈칸)    : $COMMA_REMNANTS"
echo ""

# --- 9. 결함 4: 코드 절단 ---
echo "--- 9. 결함 4: LOT/자재코드 절단 ---"
echo ""

# 슬래시 뒤 코드가 GX 포함이며 절단된 사례
GX_CODE_PRODUCTS=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations d JOIN items i ON i.id = d.source_item_id WHERE i.product_name_ko REGEXP '/ [A-Z0-9]*GX[A-Z0-9]+' AND d.material_code IS NULL")
echo "  GX 자재코드 미추출          : $GX_CODE_PRODUCTS"

MAT_CODE_NULL=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations d JOIN items i ON i.id = d.source_item_id WHERE i.product_name_ko REGEXP '/ [0-9]{6,7}' AND d.material_code IS NULL")
echo "  6~7자리 자재코드 미추출     : $MAT_CODE_NULL"

LOT_NULL_SLASH=$($MYSQL -N -e "SELECT COUNT(*) FROM declarations d JOIN items i ON i.id = d.source_item_id WHERE i.product_name_ko LIKE '%/%' AND d.material_code IS NULL AND i.product_name_ko REGEXP 'L[0-9][A-Z0-9]{4,}'")
echo "  L코드 LOT 미분리 (슬래시 케이스) : $LOT_NULL_SLASH"
echo ""

# --- 10. NORMALIZED인데 잔재가 있는 사례 ---
echo "--- 10. NORMALIZED 잔재 검사 ---"
echo ""

NORM_WITH_DEBRIS=$($MYSQL -N -e "
SELECT COUNT(*) FROM declarations
WHERE normalization_status = 'NORMALIZED'
  AND (sku_display_name_ko LIKE '%()%'
    OR sku_display_name_ko LIKE '%[]%'
    OR sku_display_name_ko LIKE '%,,%'
    OR sku_display_name_ko LIKE '%, ,%');
")
echo "  NORMALIZED + 잔재 (빈괄호/연속쉼표): $NORM_WITH_DEBRIS"
echo ""

# --- 11. 대표 결함 샘플 (최대 5건씩) ---
echo "--- 11. 결함 샘플 (각 유형별 최대 3건) ---"
echo ""

echo "[괄호 불일치 샘플]"
$MYSQL -t -e "SELECT d.rcno, d.base_product_name_ko FROM declarations d WHERE (CHAR_LENGTH(d.base_product_name_ko) - CHAR_LENGTH(REPLACE(d.base_product_name_ko, '(', ''))) != (CHAR_LENGTH(d.base_product_name_ko) - CHAR_LENGTH(REPLACE(d.base_product_name_ko, ')', ''))) LIMIT 3"
echo ""

echo "[빈 괄호 샘플]"
$MYSQL -t -e "SELECT d.rcno, d.base_product_name_ko, d.sku_display_name_ko FROM declarations d WHERE d.base_product_name_ko LIKE '%()%' OR d.sku_display_name_ko LIKE '%()%' LIMIT 3"
echo ""

echo "[코드 절단 샘플]"
$MYSQL -t -e "SELECT d.rcno, d.base_product_name_ko, i.product_name_ko AS source_ko FROM declarations d JOIN items i ON i.id = d.source_item_id WHERE i.product_name_ko REGEXP '/ [A-Z0-9]*GX[A-Z0-9]+' AND d.material_code IS NULL LIMIT 3"
echo ""

echo "============================================"
echo " 검증 하네스 완료"
echo "============================================"
