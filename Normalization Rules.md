# 원장 데이터 비파괴 정규화 규칙

## 0. 적용 범위와 조사 기준

이 문서는 MFDS 수입신고 원장을 제품 데이터로 정제할 때 적용하는 규칙이다. 원장을 수정하는 규칙이 아니라, 원장에서 파생 값을 생성하는 계약이다.

2026-08-04 로컬 원장 스냅샷을 전수 조사한 결과는 다음과 같다.

| 항목 | 값 |
|---|---:|
| 전체 관찰 이력 | 16,048행 |
| 고유 RCNO | 11,924건 |
| 반복 관찰 RCNO | 3,953건 |
| 반복 관찰 중 의미 해시 변경 | 0건 |
| 처리일 범위 | 2023-07-03 ~ 2026-08-03 |

문자열 분포와 규칙 건수는 반복 수집 횟수의 영향을 제거하기 위해 RCNO별 최신 1행을 기준으로 계산한다. 원장 이력 자체는 모든 관찰을 유지한다.

### 데이터 단위

- 원장의 식별 단위는 `RCNO` 수입신고 사건이다.
- 제품의 식별 단위는 병 단위 SKU다.
- 같은 제품·같은 에디션이라도 용량이 다르면 다른 SKU다.
- 병 하나의 용량은 SKU 식별 요소지만 박스 입수량은 물류 정보다.
- 도수·PROOF·STRENGTH·캐스크·자재코드는 문맥에 따라 SKU를 구분할 수 있다.
- 같은 SKU라도 LOT과 RCNO가 다르면 원장에는 별개의 신고 이력으로 남는다.
- 원장 정규화 키만으로 서로 다른 RCNO나 SKU를 자동 병합하지 않는다.

## 1. 정책

이 프로젝트는 **원본 보존형 비파괴 정규화(Raw-preserving Non-destructive Normalization)**를 적용한다.

- 수집한 원문은 수정하거나 덮어쓰지 않는다.
- 표준 표기와 구조화된 값은 원문에서 별도로 생성한다.
- 검색용 키는 중복 후보를 찾는 용도로만 사용한다.
- 정규화 결과만으로 서로 다른 원장을 자동 병합하지 않는다.
- 정규화 규칙의 버전을 기록하여 같은 결과를 재현할 수 있어야 한다.

## 2. 값의 책임

| 값 | 책임 |
|---|---|
| `source_raw_html` | MFDS HTML과 공백, HTML entity를 포함한 실제 원문이다. 현재 `raw_row_html`에 해당한다. |
| `parsed_source_value` | HTML에서 추출하고 공백을 정리한 소스 값이다. 현재 `product_name_ko`, `product_name_en` 등에 해당한다. |
| `display` | 단위 표기 등을 정돈하여 사용자에게 보여주는 값이다. |
| `structured` | 용량·도수·숙성연수·LOT처럼 확정 가능한 값을 타입으로 분리한다. |
| `search_key` | 검색과 중복 후보 탐색을 위해 제한적으로 표준화한 값이다. |

`product_name_ko`와 `product_name_en`은 실제 원문이 아니다. 목록 파서가 이미 앞뒤 공백을 제거하고 연속 공백을 한 칸으로 줄인 값이다. 실제 원문 증거는 `raw_row_html`과 원문 해시다.

어떤 파생 값도 `source_raw_html`이나 `parsed_source_value`를 덮어쓰지 않는다. 정제할 수 없는 값은 원문을 유지하고 검토 대상으로 표시한다.

## 3. 공통 문자열 규칙

현재 파서는 `parsed_source_value`를 만들 때 다음 작업을 이미 수행한다.

1. 문자열 앞뒤 공백을 제거한다.
2. 연속된 공백은 한 칸으로 줄인다.

실제 조사에서 앞뒤 공백, 연속 ASCII 공백, tab, CR/LF, NBSP, 전각 공백, 깨진 대체문자는 모두 0건이었다. 하위 정제에서 같은 공백 정리를 다시 수행해도 되지만 원장 보존 기능으로 설명하지 않는다.

표시값에는 다음 규칙을 적용한다.

1. 의미를 가진 특수문자와 영문 대소문자를 유지한다.
2. 단위 토큰의 철자만 표준화한다.
3. 빈 문자열과 `-`는 필드별 결측 규칙에 따라 해석하되 소스 값은 보존한다.

검색용 키에는 다음 규칙을 추가할 수 있다.

- Unicode 호환 문자를 통일하되 표시값은 유지한다.
- 영문 대소문자를 구분하지 않는다.
- 굽은 따옴표 `’`, `‘`를 직선 따옴표 `'`로 통일한다.
- 다양한 대시 문자를 하이픈 `-`으로 통일한다.
- `스트렝스/스트랭스`, `쉐리/셰리`, `피니시/피니쉬`, `배럴/바렐` 같은 음차 차이는 영문명이 같을 때만 검색 후보 단계에서 통일한다.

검색용 키가 같다는 사실은 동일 제품 판정 근거가 아니다.

## 4. 특수문자 규칙

다음 문자는 제품 구분 정보를 포함할 수 있으므로 제거하지 않는다.

| 문자 | 용도 예시 |
|---|---|
| `'`, `’` | 브랜드명, 소유격 |
| `#`, `No.` | 캐스크·배치·에디션 번호 또는 미확정 변형 마커 |
| `(`, `)` | 구형, 에디션, 추가 설명 |
| `-` | 합성어, 제품명 구성 |
| `&` | 브랜드 또는 캐스크 조합 |
| `%` | 알코올 도수 |
| `*` | 묶음 수량과 구성 |
| `/` | 지역 또는 복합 표기 |
| `[` , `]` | 용량, LOT, 성분 설명 |
| `+` | 제품 조합 또는 제품명 |
| `@` | 시리즈 번호 |
| `™` | 상표 표기 |

특수문자를 모두 제거한 값은 검색 후보 생성에만 사용할 수 있다. 동일성 판정이나 자동 병합 기준으로 사용하지 않는다.

실제 원장에는 `@001`, `™`, 전각 `＆`, `㈜`, 대괄호가 존재한다. 특수문자는 문자 자체가 아니라 주변 문맥으로 해석한다.

## 5. 필드별 규칙

### 5.1 제품명

- 한글명과 영문명을 각각 보존한다.
- 영문 표기와 숫자는 한글명에 포함되어 있어도 제거하지 않는다.
- 영문명의 대소문자는 표시값에서 유지한다.
- 이름에 포함된 용량·도수·숙성연수·LOT은 명확한 문맥일 때만 구조화 값으로 추출한다.
- 구조화한 토큰도 소스 이름에서는 제거하지 않는다.
- SKU 표시명에서는 확정된 기술 주석만 분리할 수 있다.
- 괄호 전체를 일괄 제거하지 않는다.
- 한글 `구형`은 패키지·라벨 세대 주석으로 분리하되 영문 `OLD`와 연결하지 않는다.

실제 괄호 사용은 용량 604건, 코드형 529건, 명시 LOT·제조번호 160건, 한글 설명 153건, 도수형 146건, 복합·기타 176건으로 용도가 다르다.

다음 괄호는 제품 정체성의 일부이므로 이름에 유지한다.

```text
(서울에디션)
(태평구룡호)
(햄든 디오케이)
(가정용)
```

다음 값은 구조화 후보지만 한 괄호 안에 값이 섞여 있으므로 내부 토큰 단위로 파싱한다.

```text
(1920ml, L A5B)
(35%,700ml)
(700ml, L N5D)
```

최신 RCNO 11,924건에서 한글 `구형`은 6건이고 `신형`, `리뉴얼`은 0건이었다. 영문 단어 `OLD`는 272건이지만 `구형`과 직접 대응한 사례는 0건이다. `OLD`는 `15YEAR OLD` 같은 숙성 표현 또는 `OLD FORESTER`, `OLD PARR` 같은 고유명으로 유지한다.

예시:

```text
술 이름 700ML
parsed_source_name = "술 이름 700ML"
base_product_name  = "술 이름"
sku_display_name   = "술 이름 700ml"
name_search_key    = "술 이름 700ml"
volume_ml          = 700
```

### 5.2 용량

- 표준 표시 단위는 소문자 `ml`을 사용한다.
- 구조화 값은 정수형 `volume_ml`로 저장한다.
- 같은 제품·에디션이라도 `volume_ml`이 다르면 다른 SKU다.
- 용량 미상 SKU를 용량이 확인된 SKU와 자동 병합하지 않는다.
- 숫자만 있는 값은 용량으로 해석하지 않는다.
- 묶음은 `unit_volume_ml`과 `package_count`로 분리한다.
- 병 단위 SKU 후보 키에는 `unit_volume_ml`만 사용하고 `package_count`는 포함하지 않는다.
- `package_count`는 소스에 표시된 포장·선적 수량으로 보존하며 소매 세트 여부가 별도로 확인되지 않으면 물류 정보로 취급한다.
- 용량이 크다는 이유만으로 오류 처리하지 않는다.

실제 용량 표기는 `mL` 929건, `ml` 618건, `ML` 566건, `L` 189건이다. 용량 토큰이 확인된 RCNO는 2,319건이고 9,605건에는 용량이 없다.

숫자 파서는 다음 순서로 처리한다.

1. 숫자 경계를 확인한다.
2. `4,000` 같은 천 단위 구분 쉼표를 제거한다.
3. 소수 리터를 허용한다.
4. 단위 `mL`, `ml`, `ML`을 `ml`로 통일한다.
5. `L`은 1,000을 곱해 `ml` 정수로 변환한다.
6. 앞뒤 토큰을 확인하여 LOT 번호 일부를 용량으로 오인하지 않는다.

| 원문 | 구조화 값 | 표준 표시 |
|---|---:|---|
| `700ml` | `700` | `700ml` |
| `700ML` | `700` | `700ml` |
| `1L` | `1000` | `1000ml` |
| `1.75L` | `1750` | `1750ml` |
| `4,000ml` | `4000` | `4000ml` |
| `173.3L` | `173300` | `173300ml` |
| `250ML*2` | 단위 `250`, 수량 `2` | `250ml × 2` |

`4,000ml`을 단순 숫자 정규식으로 읽으면 `000ml`로 오인할 수 있다. `X숫자`만으로 묶음을 찾으면 `053491GX0700612` 같은 LOT 코드를 수량으로 오인한다. 묶음은 반드시 `용량 + X/* + 수량` 문맥으로 제한한다.

실제 원장에는 같은 병 용량이 `125ml × 24/48`, `250ml × 20/30/36`으로 달라지는 사례가 있다. 박스 수량을 키에 넣으면 같은 병 SKU를 여러 제품으로 과분할하므로 사용하지 않는다.

### 5.3 알코올 도수

- 구조화 값은 백분율 숫자 `abv_percent`로 저장한다.
- 표준 표시에는 `%`를 붙인다.
- 불필요한 소수점 이하 0은 제거한다.
- 한글명과 영문명에 도수 후보가 모두 있으면 숫자 일치 여부를 확인한다.
- 두 값이 다르면 자동 확정하지 않는다.
- 확정된 `abv_percent`, `proof_value`, `CASK/BARREL STRENGTH`는 SKU 후보를 구분하는 사양 축으로 사용한다.
- 도수가 다른 후보는 이름과 용량이 같아도 자동 병합하지 않는다.
- 자동 범위를 벗어난 값은 오류로 버리지 않고 검토 대상으로 보낸다.
- 성분 함량은 `ingredient_percent_raw`, `ingredient_percent`로 분리하고 `abv_percent`에 넣지 않는다.
- 성분 퍼센트가 여러 개면 모든 퍼센트 원문만 `ingredient_percent_raw`에 보존하고 숫자 컬럼은 비우며 검토 대상으로 보낸다.

```text
46.00%  -> 46%
57.10%  -> 57.1%
```

#### 자동 추출 가능한 패턴

| 소스 | 패턴 | 조사 건수 | 처리 |
|---|---|---:|---|
| 영문명 | `(40%)` | 540 | 자동 추출 |
| 영문명 | 이름 끝 `40%` | 502 | 자동 추출 |
| 영문명 | `40% VOL`, `53%VOL` | 3 | 자동 추출 |
| 영문명 | `ALC.40%`, `WHITE SPIRIT 52%` | 12 | 명시 도수 문맥일 때 자동 추출 |
| 한글명 | `(43%)` | 397 | 자동 추출 |
| 한글명 | `주도38%` | 44 | 자동 추출 |
| 한글명 | `56도` | 43 | 자동 추출 |
| 한글명 | 이름 끝 `52%` | 38 | 성분 단어가 없을 때 자동 추출 |
| 한·영 이름 | `주도NN%`, `(NN%, 용량)`, `NN% 용량`, 이름 앞 `NN%VOL` | 관측 문맥 | 실제 도수 앵커가 있을 때 자동 추출 |

현재 자동 추출 패턴에서 관찰된 범위는 대체로 4~69%다. `0 < 값 <= 70`은 자동 처리 가드로 사용할 수 있지만, 범위 밖의 값은 무효가 아니라 `review_required`다.

#### 검토하거나 제외할 패턴

| 패턴 | 조사 건수 | 처리 |
|---|---:|---|
| 영문명 중간 `52.3% 670ML` | 13 | 용량이 바로 이어지는 문맥이면 자동 추출 |
| 영문 `100% RYE/ISLAY/POIRE` | 4 | 도수 아님 |
| 한글명 중간 단일 `%` | 73 | 검토 |
| 한글명에 `%`가 여러 개 | 26 | 성분 가능성이 높아 검토 |
| 향·과즙·농축·원액·함유 문맥 | 12 | 도수로 추출하지 않음 |
| 한글 `100%` | 3 | 마케팅·원재료 표기 |
| 숫자 `100 PROOF` | 9 | `proof`로 보존하고 자동 환산하지 않음 |
| `BARREL STRENGTH`, `OVERPROOF` | 관측 문맥 | 제품 설명이며 숫자 도수 아님 |

한·영 이름에 모두 `%`가 있는 144건 중 140건은 숫자가 일치했다. 불일치 4건은 모두 다음 유형이었다.

```text
한글명: 설원 인삼송이주(인삼0.45%, 송이0.1%) 150ml
영문명: LOW LIQUOR 42%
```

한글명의 첫 `%`를 사용하면 도수가 `0.45%`로 오수집된다. 인삼·향료·과즙·농축액·원액·추출물·주스·시럽·곡물 등 성분 문맥을 먼저 분리하고, `주도`, 용량 결합, `%VOL` 같은 실제 도수 앵커를 우선한다.

`CASK STRENGTH`, `BARREL STRENGTH`와 관측 오타 `STRENGHT`, `STRENGH`, `STRENCH`는 각각 canonical `STRENGTH`로 저장한다. `CS`는 반대 언어에 `CASK STRENGTH` 또는 명확한 한글 캐스크·배럴 스트렝스가 있을 때만 `strength_type`으로 확정한다. 단독 `CS`는 `STRENGTH_ABBREVIATION` 변형 마커와 검토 사유로 남긴다. 실측 근거가 없는 `FULL STRENGTH`, `N°`, `°` 규칙은 추가하지 않는다.

실제 데이터에는 기본명·용량·숙성이 같고 도수만 다른 그룹이 27개 있다. 카발란 솔리스트 올로로소 쉐리캐스크는 51.6~57.1%, 금문고량주 600ml는 38%와 58%로 나뉘므로 도수 축을 생략하면 서로 다른 제품 후보가 병합된다.

### 5.4 숙성연수

- 명확한 숙성 단위가 붙은 정수만 `age_years`로 저장한다.
- `-`, 빈 문자열, `NULL`은 미상으로 처리하며 0년으로 해석하지 않는다.
- 한글 `12년`, 영문 `12YO`, `12 YEARS OLD`, `AGED 12`를 자동 추출한다.
- 네 자리 숫자는 빈티지·연도·용량일 수 있으므로 숙성연수로 추출하지 않는다.
- 명시 LOT·제조번호와 라벨 없는 LOT 구간을 먼저 분리한 뒤 남은 이름에서 빈티지를 탐색한다.

실제 조사 결과는 한글 `숫자+년` 3,396건, 영문 `YO` 2,274건, `YEAR/YEARS` 870건, `AGED` 197건이다. 네 자리 숫자는 한글명 417건, 영문명 286건이며 다음처럼 의미가 섞인다.

```text
DT 언 아일라 2008       -> 빈티지 후보
가이아나 2004           -> 빈티지 후보
가쿠빈 1920ML           -> 용량
```

네 자리 숫자는 결정적 범위 `1950..2026` 안에서만 `vintage_year` 후보로 보존하고 검토 사유를 유지한다. 여러 타당 연도가 있으면 가장 오래된 연도를 선택하고 전체 후보를 검토 근거에 남긴다. `1500`, `1792`, `1800`, `1907`, `1920`, `1942`, `2099` 같은 브랜드·비현실 범위 숫자와 `발베니 12년-700ML(제조번호 : L 0113953 2006)`의 LOT 내부 숫자는 빈티지로 사용하지 않는다.

### 5.5 LOT·제조번호·코드

LOT은 제품 SKU가 아니라 특정 제조 배치를 식별하는 번호다. LOT이 달라도 제품명·에디션·용량이 같으면 같은 SKU일 수 있다. RCNO 원장 행은 계속 분리한다.

실제 원장에는 `LOT NO.`, `LOTE`, `제조번호`가 명시된 RCNO가 171건 있다.

```text
몽키숄더-700ML(LOT NO. L 0111613 2605)
몽키숄더-700ML(LOT NO. L 0113949 1606)

발베니 12년-700ML(제조번호 : L 0109180 0404)
발베니 12년-700ML(제조번호 : L 0111534 1605)
```

명시 라벨이 있는 값은 `lot_number` 또는 `manufacture_number`로 분리하고 검색 가능하게 유지한다.

라벨 없는 코드는 형태와 문맥에 따라 LOT, 자재코드, 캐스크 번호로 나눈다. 숫자나 접두사만 보고 일괄 제거하지 않는다.

| 코드 유형 | 처리 | 실제 근거 |
|---|---|---|
| 명시 `LOT NO.`, `LOTE`, `제조번호` | LOT·제조번호로 분리, SKU 키 제외 | 최신 RCNO 171건 |
| 이름 접미 `L`+숫자 계열 | LOT 후보로 분리, SKU 키 제외 | 375행·30개 제품군에서 과분할 발생 |
| 괄호 안 6자리 숫자 | 수입사 자재코드 후보, SKU 사양에 유지 | 코드별 수입사·제품명·용량이 반복적으로 고정 |
| 슬래시 뒤 6~7자리 숫자 | 자재코드 후보, SKU 사양에 유지 | 1,470행·10개 수입사에서 반복 |
| SMWS `NNNNNNGX...` | 단일 캐스크 제품 식별 코드로 유지 | 코드가 다른 30종이 기본명 하나로 수렴 |
| `#숫자`, `No. 숫자`와 `SINGLE CASK/CASK` 문맥 | 캐스크 번호로 유지 | 같은 도수에서도 `#9485`, `#9482`가 제품을 구분 |
| `#숫자`, `No. 숫자`와 `BATCH/EDITION` 문맥 | 해당 배치·에디션 필드로 유지 | 문맥과 숫자 원문을 함께 보존 |
| `@숫자`, `SERIES 숫자` | `SERIES_NUMBER` 변형 마커로 유지 | 시리즈 번호가 제품을 구분할 수 있음 |
| 숫자 마커의 기타 문맥 | `UNKNOWN` 변형 마커와 검토 | 미확정 마커를 이름·SKU 후보 키에서 제거하지 않음 |
| `BATCH n` | 숫자만 배치로 유지 | 한 언어만 명확해도 보존하며 양쪽 숫자가 다를 때만 검토 |
| `SMALL BATCH` | 제품명으로 유지 | 숫자 없는 제품 설명을 배치 번호로 해석하지 않음 |

예시:

```text
조니워커 블랙 레이블 700mL (778061) L5293CA009
-> material_code = 778061
-> lot_number = L5293CA009

싱글 몰트 스카치 위스키 / 135066GX0700615
-> material_code = 135066GX0700615
-> 코드 제거 금지

글렌알라키 싱글캐스크 (59.1%) (#9485)
-> cask_number = 9485
```

같은 LOT 코드가 여러 RCNO에 나타날 수 있으므로 LOT과 RCNO는 1:1 관계가 아니다. 반대로 6~7자리 자재코드와 단일 캐스크 번호는 제품 후보를 구분할 수 있으므로 LOT으로 제거하면 안 된다.

`BATCH PROOF`, 숫자 `PROOF`, `CASK/BARREL STRENGTH`, 캐스크 번호, 자재코드, 에디션명은 판매 제품을 구분할 수 있으므로 LOT 제거 규칙을 적용하지 않는다. 에디션 qualifier는 에디션 직전의 제한된 토큰만 캡처하며 이름 전체를 greedy 캡처하지 않는다. 한 언어에만 명확한 에디션이 있어도 보존하고 양쪽의 실제 숫자 값이 충돌할 때만 검토한다.

### 5.6 소비기한

`expiry_text`는 원문을 유지하면서 `expiry_start`와 `expiry_end`로 분리한다.

| 원문 형태 | 건수 | 구조화 결과 |
|---|---:|---|
| `-` | 8,643 | 시작·종료 모두 `NULL` |
| `2025-06-03 ~` | 3,078 | 시작일만 설정, 종료일 `NULL` |
| `2026-02-27 ~ 2026-08-26` | 198 | 시작일과 종료일 설정 |
| `~ 2026-08-26` | 5 | 시작일 `NULL`, 종료일만 설정 |

`날짜 ~`는 종료일이 열린 범위이고 `~ 날짜`는 시작일이 열린 범위다. 명칭을 반대로 사용하지 않는다. `-`를 무기한 또는 영구 유효로 해석하지 않는다.

### 5.7 업체명과 국가

수입업체명은 원문을 유지하고 법인 표기만 별도 속성으로 분리할 수 있다.

| 표기 | 건수 |
|---|---:|
| `주식회사` | 4,634 |
| `(주)` | 3,835 |
| `㈜` | 1 |
| `유한회사` | 730 |

법인 접미사를 제거한 검색 키는 후보 탐색에만 사용한다. 회사명이 비슷하다는 이유로 자동 병합하지 않는다.

제조국과 수출국이 다른 RCNO는 5,006건으로 전체의 41.98%다. 이는 물류 경로를 반영할 수 있으므로 두 필드를 합치거나 한쪽 값으로 보정하지 않는다.

### 5.8 품목과 자유 형식 속성

현재 `item_name`은 위스키·브랜디·일반증류주·리큐르 4개 값이며 조회 품목과 불일치가 0건이다. 현재 단계에서 별도의 카테고리 교정 사전은 필요하지 않다.

캐스크 설명은 제품명에 포함될 수 있는 자유 형식 정보이며 초기 단계부터 enum으로 제한하지 않는다. 다만 `SINGLE CASK` 문맥의 명시 번호는 `cask_number`로 분리한다. 캐스크·피니시·에디션 구조화는 제품명 정규화 이후 별도 단계에서 수행한다.

## 6. 중복 후보 처리

정규화 키가 같다는 것은 문자열이 유사하다는 뜻이지 동일 제품이라는 뜻이 아니다.

```text
Writers' Tears Copper Pot
Writer's Tears Copper Pot

잭다니엘스 싱글 배럴 100프루프
잭 다니엘스 싱글배럴 100 프루프
```

위와 같은 값은 중복 후보로 표시할 수 있지만 자동 병합하지 않는다. 최종 동일성 판단은 별도의 검토 또는 추가 식별 근거를 사용한다.

특수문자·괄호·용량을 공격적으로 제거한 검색 키는 실제 조사에서 용량과 도수가 다른 제품을 같은 키로 합쳤다.

```text
가쿠빈 700ml
가쿠빈 1920ml

메이커스 마크 200ml
메이커스 마크 375ml
메이커스 마크 750ml
메이커스 마크 1L
```

규칙은 다음과 같다.

- 용량이 다르면 다른 SKU다.
- 같은 용량이라고 동일 SKU가 확정되는 것은 아니다.
- 에디션·도수·캐스크·제품 설명이 다르면 별도 제품일 수 있다.
- LOT만 다르면 같은 SKU 후보가 될 수 있지만 RCNO 이력은 병합하지 않는다.
- 용량 미상 값은 용량이 확인된 SKU에 자동 병합하지 않는다.
- 키1이 `위스키`, `보드카`처럼 품목 총칭에 불과하면 제품 후보 키 생성을 중단하고 검토한다.
- 제조국이 다르면 키에 넣지는 않지만 자동 병합을 차단한다.

### 6.1 기획자용 3키 표현

기획자 화면에서는 `원본명 | 정제 이름 | 키1 | 키2 | 키3`으로 단순화한다. 물리 스키마는 키3의 구성 요소를 개별 컬럼으로 저장한다.

| 화면 항목 | 의미 | 물리 값 |
|---|---|---|
| 키1: 제품군 | 브랜드·제품 본체와 의미 있는 시리즈명 | `base_product_name_*`, `name_search_key_*` |
| 키2: 병 규격 | 병 하나의 확정 용량 | `unit_volume_ml` |
| 키3: 제품 변형 | 숙성·빈티지·도수·버전·에디션·캐스크·자재코드의 선택적 묶음 | 각 구조화 컬럼의 canonical tuple |

키1~3은 모두 후보 탐색용이며 unique key가 아니다. 키3을 문자열 하나로 저장하지 않고 다음 값을 독립적으로 보존한다.

```text
age_years
vintage_year
abv_percent
proof_value
strength_type
version_marker
edition_name
material_code
cask_number
batch_number
```

RCNO, LOT·제조번호, 박스 입수량은 제품 식별 키에서 제외한다. 제조국·수출국·수입사는 후보 검토 근거로만 사용한다.

#### 기획자용 예시

| 원본명 | 정제 이름 | 키1: 제품군 | 키2: 병 규격 | 키3: 제품 변형 | 키 제외 정보 |
|---|---|---|---|---|---|
| `글렌알라키 15년 구형` | 글렌알라키 15년 구형 | 글렌알라키 | 미확인 | 15년·구형 | RCNO |
| `GLENALLACHIE 15YEAR OLD` | 글렌알라키 15년 | 글렌알라키 | 미확인 | 15년 | `OLD`를 구형으로 사용하지 않음 |
| `카발란 솔리스트 올로로소 쉐리캐스크 (56.3%)` | 카발란 솔리스트 올로로소 쉐리 캐스크 56.3% | 카발란 솔리스트 올로로소 쉐리 캐스크 | 미확인 | ABV 56.3% | RCNO |
| `발베니 14년-700ML(LOT NO. L 0111349 1905)` | 발베니 14년 700ml | 발베니 | 700ml | 14년 | LOT 전체 |
| `조니워커 블랙 레이블 700mL (778061) L5293CA009` | 조니워커 블랙 레이블 700ml | 조니워커 블랙 레이블 | 700ml | 자재코드 778061 | LOT `L5293CA009` |
| `싱글 몰트 스카치 위스키 / 135066GX0700615` | 싱글 몰트 스카치 위스키 | 총칭명으로 검토 | 700ml 후보 | 자재·캐스크 코드 135066GX0700615 | RCNO |
| `잭다니엘스 싱글배럴 100 프루프` | 잭다니엘스 싱글 배럴 100 프루프 | 잭다니엘스 싱글 배럴 | 미확인 | 100 PROOF | RCNO |
| `고량주 250ML × 20` | 고량주 250ml | 고량주 | 250ml | 미확인 | 박스 입수량 20 |
| `KILCHOMAN 100% ISLAY` | KILCHOMAN 100% ISLAY | KILCHOMAN 100% ISLAY | 미확인 | 미확인 | `100%`를 ABV로 사용하지 않음 |

### 6.2 다중 모델 실측 검증

2026-08-04에 OpenCode, Sonnet, Opus, Terra가 같은 로컬 원장을 읽기 전용으로 교차 검증했다. 최종 판정은 실제 쿼리로 재현된 교집합과 적대적 반례를 우선한다.

| 검증 항목 | 실측 결과 | 결론 |
|---|---:|---|
| 한글 `구형` | 최신 RCNO 6건 | 한글 전용 버전 주석 |
| `구형`과 영문 `OLD` 직접 대응 | 0건 | 자동 번역·치환 금지 |
| 영문 `OLD` | 272건 | 숙성 표현 또는 고유명 |
| `EDITION/에디션` 합집합 | 122건, 양쪽 동시 71건 | 한 언어만 명확해도 보존하고 값 충돌만 검토 |
| 같은 키에서 도수만 다른 제품군 | 27개 | ABV·PROOF·STRENGTH 축 필요 |
| 동일 영문명의 복수 용량 | 118개 이름군·1,265 RCNO | 병 용량 키 필요 |
| 라벨 없는 LOT에 의한 과분할 | 375행·30개 제품군 | 접미 `L` 코드 분리 |
| SMWS 단일 캐스크 코드 | 30종 | 캐스크 코드 제거 금지 |
| 용량 미확인 | 9,592건·80.4% | 결측값 자동 병합 금지 |
| `스트렝스/스트랭스` | 101건/71건 | 영문 동일 시 검색키 통일 |
| `쉐리/셰리` | 177건/55건 | 영문 동일 시 검색키 통일 |
| `피니시/피니쉬` | 25건/7건 | 영문 동일 시 검색키 통일 |

검증 과정에서 기존 3키만 적용하면 SMWS 단일 캐스크 30종이 하나로 오병합되고, LOT 코드를 그대로 남기면 375행이 과분할되는 반례가 확인됐다. 따라서 키 개수보다 구조화 속성의 문맥 판정과 `review_required` 우선 원칙을 사용한다.

## 7. 권장 정제 결과

```text
source_raw_html
source_raw_sha256
parsed_source_name_ko
parsed_source_name_en
base_product_name
sku_display_name
name_search_key
volume_raw
volume_ml
unit_volume_ml
package_count
abv_raw
abv_percent
ingredient_percent_raw
ingredient_percent
proof_raw
proof_value
strength_type
age_raw
age_years
vintage_year
version_marker
edition_name
variant_marker_raw
variant_marker_type
variant_marker_value
material_code
cask_number
batch_number
lot_number
manufacture_number
expiry_raw
expiry_start
expiry_end
importer_raw
importer_base_name
legal_entity_type
normalization_status
normalization_version
normalization_reasons
```

`normalization_status`는 최소한 다음 값을 구분한다.

| 상태 | 의미 |
|---|---|
| `PENDING` | 아직 정제하지 않은 초기 상태이며 컬럼 기본값 |
| `NORMALIZED` | 안정된 규칙으로 구조화 완료 |
| `PARTIAL` | 일부 값만 구조화하고 나머지는 원문 유지 |
| `REVIEW_REQUIRED` | 충돌 또는 모호한 문맥이 있어 검토 필요 |
| `UNPARSED` | 지원하지 않는 형식 |
| `STALE` | 원본이 갱신되어 재정제가 필요 |

`normalization_reasons`에는 적용 규칙과 검토 사유를 코드 형태로 남긴다.

사유 코드는 검토를 유발하는 코드와 정보성 코드로 나뉜다. 정보성 코드만 남은 결과는 `normalized`이며 `review_status`를 `PENDING`으로 올리지 않는다.

```text
VOLUME_MISSING
VOLUME_UNIT_CASE_NORMALIZED
VOLUME_THOUSANDS_SEPARATOR
ABV_CONFLICT_BETWEEN_LANGUAGES
ABV_COMPOSITION_CONTEXT
LOT_UNLABELED_CODE
LOT_SUFFIX_CODE_EXCLUDED_FROM_SKU
MATERIAL_CODE_PRESERVED_FOR_SKU
CASK_NUMBER_PRESERVED_FOR_SKU
PACKAGE_COUNT_EXCLUDED_FROM_BOTTLE_SKU
KO_VERSION_MARKER_WITHOUT_ENGLISH_MAPPING
INGREDIENT_PERCENT_MULTIPLE_VALUES
VARIANT_MARKER_AMBIGUOUS
STRENGTH_ABBREVIATION_AMBIGUOUS
BATCH_VALUE_CONFLICT
EDITION_VALUE_CONFLICT
GENERIC_PRODUCT_NAME_REVIEW_REQUIRED
PARENTHESIS_SEMANTIC_TEXT
```

## 8. 정제 테이블 스키마

### 8.1 테이블 역할

`mfds_declarations`는 RCNO별 정제 결과를 관리하는 현재 상태 테이블이다.

- `mfds_items`: 같은 RCNO의 반복 관찰을 모두 보존하는 불변 원장 이력
- `mfds_declarations`: RCNO당 1행인 원본 참조와 정제 결과
- `source_item_id`: 정제 근거로 선택한 최신 `mfds_items.id`
- `mfds_declaration_details`: 원본과 정제 결과를 함께 보여주는 조회 View
- 정제 컬럼: 소스 값에서 파생한 제품·용량·도수·숙성·LOT 정보
- 검토 컬럼: 자동 정제 상태와 휴먼 리뷰 결과

원본 값은 `mfds_declarations`에 복제하지 않는다. 일반 조회에서는 `mfds_declaration_details` View를 사용하고, 원본 관찰 이력 전체가 필요할 때만 `mfds_items`를 직접 조회한다.

FK는 두지 않는다. `source_item_id`는 추적용 논리 참조이며 RCNO 유일성만 DB에서 강제한다. `mfds_items`는 불변 원장이므로 참조된 행을 수정하거나 삭제하지 않는다.

### 8.2 DDL

```sql
CREATE TABLE mfds_declarations
(
    id                                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    rcno                              VARCHAR(32) NOT NULL,
    source_item_id                    BIGINT UNSIGNED NOT NULL,

    base_product_name_ko              TEXT NULL,
    base_product_name_en              TEXT NULL,
    sku_display_name_ko               TEXT NULL,
    sku_display_name_en               TEXT NULL,
    name_search_key_ko                TEXT NULL,
    name_search_key_en                TEXT NULL,
    sku_candidate_key_sha256          BINARY(32) NULL,

    volume_raw                        VARCHAR(255) NULL,
    volume_ml                         INT UNSIGNED NULL,
    unit_volume_ml                    INT UNSIGNED NULL,
    package_count                     INT UNSIGNED NULL,
    abv_raw                           VARCHAR(255) NULL,
    abv_percent                       DECIMAL(6, 3) NULL,
    ingredient_percent_raw            TEXT NULL,
    ingredient_percent                DECIMAL(6, 3) NULL,
    proof_raw                         VARCHAR(255) NULL,
    proof_value                       DECIMAL(7, 3) NULL,
    strength_type                     VARCHAR(64) NULL,
    age_raw                           VARCHAR(255) NULL,
    age_years                         SMALLINT UNSIGNED NULL,
    vintage_raw                       VARCHAR(255) NULL,
    vintage_year                      SMALLINT UNSIGNED NULL,
    version_marker                    VARCHAR(64) NULL,
    edition_name                      TEXT NULL,
    variant_marker_raw                VARCHAR(255) NULL,
    variant_marker_type               VARCHAR(64) NULL,
    variant_marker_value              VARCHAR(255) NULL,
    material_code                     VARCHAR(255) NULL,
    cask_number                       VARCHAR(255) NULL,
    batch_number                      VARCHAR(255) NULL,
    lot_number                        TEXT NULL,
    manufacture_number                TEXT NULL,

    expiry_raw                        TEXT NULL,
    expiry_start                      DATE NULL,
    expiry_end                        DATE NULL,
    importer_base_name                TEXT NULL,
    importer_search_key               TEXT NULL,
    legal_entity_type                 VARCHAR(64) NULL,
    overseas_establishment_search_key TEXT NULL,

    alcohol_name_ko                   TEXT NULL,
    alcohol_name_en                   TEXT NULL,
    alcohol_category_ko               VARCHAR(64) NULL,
    alcohol_category_en               VARCHAR(64) NULL,
    alcohol_region_ko                 VARCHAR(128) NULL,
    alcohol_region_en                 VARCHAR(128) NULL,
    alcohol_abv                       VARCHAR(32) NULL,
    cask_candidate                    VARCHAR(255) NULL,
    distillery_name_ko_candidate      TEXT NULL,
    distillery_name_en_candidate      TEXT NULL,
    manufacture_country_name_ko       VARCHAR(128) NULL,
    manufacture_country_name_en       VARCHAR(128) NULL,
    manufacture_country_alpha2        CHAR(2) NULL,
    manufacture_country_alpha3        CHAR(3) NULL,
    export_country_name_ko            VARCHAR(128) NULL,
    export_country_name_en            VARCHAR(128) NULL,
    export_country_alpha2             CHAR(2) NULL,
    export_country_alpha3             CHAR(3) NULL,

    normalization_status              VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    normalization_version             VARCHAR(64) NULL,
    normalization_reasons             JSON NOT NULL DEFAULT (JSON_ARRAY()),
    unparsed_fragments_json           JSON NOT NULL DEFAULT (JSON_ARRAY()),
    normalized_at                     DATETIME(6) NULL,

    claim_owner                       VARCHAR(100) NULL,
    claim_lease_until                 DATETIME(6) NULL,
    claim_attempts                    INT UNSIGNED NOT NULL DEFAULT 0,
    claim_next_attempt_at             DATETIME(6) NULL,
    claim_last_error                  TEXT NULL,

    review_status                     VARCHAR(32) NOT NULL DEFAULT 'NOT_REQUIRED',
    reviewed_by                       VARCHAR(255) NULL,
    reviewed_at                       DATETIME(6) NULL,
    review_note                       TEXT NULL,
    created_at                        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at                        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),

    PRIMARY KEY (id),
    UNIQUE KEY uk_declarations_rcno (rcno),
    KEY idx_declarations_source_item (source_item_id),
    KEY idx_declarations_sku_candidate (sku_candidate_key_sha256, unit_volume_ml),
    KEY idx_declarations_normalization (normalization_status, updated_at),
    KEY idx_declarations_claim (normalization_status, claim_next_attempt_at, claim_lease_until),
    KEY idx_declarations_review (review_status, updated_at),
    KEY idx_declarations_importer (importer_base_name(191)),
    KEY idx_declarations_alcohol_name (alcohol_name_ko(191)),
    KEY idx_declarations_country (manufacture_country_alpha2, export_country_alpha2)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC COMMENT='RCNO별 원본 참조와 비파괴 정제 결과';

CREATE VIEW mfds_declaration_details AS
SELECT d.*,
       i.job_id                        AS source_job_id,
       i.task_id                       AS source_task_id,
       i.fetch_id                      AS source_fetch_id,
       i.row_no                        AS source_row_no,
       i.queried_item_code             AS source_queried_item_code,
       i.queried_item_name             AS source_queried_item_name,
       i.product_division_name         AS source_product_division_name,
       i.importer_name                 AS source_importer_name,
       i.product_name_ko               AS source_product_name_ko,
       i.product_name_en               AS source_product_name_en,
       i.item_name                     AS source_item_name,
       i.overseas_establishment_name   AS source_overseas_establishment_name,
       i.processed_date_raw            AS source_processed_date_raw,
       i.processed_date                AS source_processed_date,
       i.expiry_text                   AS source_expiry_text,
       i.manufacture_country_name      AS source_manufacture_country_name,
       i.export_country_name           AS source_export_country_name,
       i.detail_href                   AS source_detail_href,
       i.canonical_values_json         AS source_canonical_values_json,
       i.raw_row_html                  AS source_raw_row_html,
       i.raw_row_sha256                AS source_raw_row_sha256,
       i.semantic_sha256               AS source_semantic_sha256,
       i.parser_version                AS source_parser_version,
       i.parser_warning                AS source_parser_warning,
       i.observed_at                   AS source_observed_at
FROM mfds_declarations AS d
JOIN mfds_items AS i ON i.id = d.source_item_id;
```

### 8.3 컬럼 계약

#### 원본 참조 컬럼

- `source_item_id`는 RCNO의 최신 관찰 `mfds_items.id`다.
- `source_item_id`가 가리키는 `mfds_items.rcno`는 `mfds_declarations.rcno`와 같아야 한다.
- 원본 컬럼은 `mfds_declaration_details` View에서 `source_*` 이름으로 노출한다.
- View의 원본 HTML과 해시는 `mfds_items` 값을 그대로 보여주며 복제하거나 재생성하지 않는다.
- FK가 없으므로 애플리케이션과 검증 쿼리에서 고아 참조가 0건인지 확인한다.

#### 제품 식별 컬럼

- `base_product_name_*`는 확정된 용량·도수·LOT 같은 기술 토큰과 별도 구조화한 변형 속성을 분리한 제품 본체 이름이다.
- `sku_display_name_*`는 베이스 제품명, 확정된 제품 변형과 정규화된 병 용량을 포함한 표시명이다.
- 원문에서 분리한 에디션·버전·캐스크 표현은 구조화 컬럼과 `sku_display_name_*`에 보존하며 의미를 삭제하지 않는다.
- `sku_candidate_key_sha256`는 SKU 후보 그룹 검색용이며 unique key가 아니다.
- `sku_candidate_key_sha256` 후보에는 제품군, `unit_volume_ml`, 숙성·빈티지·도수·PROOF·STRENGTH·버전·에디션·자재·캐스크·배치와 보존된 변형 마커를 사용한다.
- `variant_marker_raw/type/value`는 `#`, `@`, `No.` 숫자와 단독 `CS`의 원문·문맥·값을 분리한다. `UNKNOWN`, `STRENGTH_ABBREVIATION`은 `base_product_name_*`, `alcohol_name_*`, 후보 키에서 제거하지 않는다.
- `ingredient_percent_raw/ingredient_percent`는 성분 함량 전용이며 `abv_raw/abv_percent`와 섞지 않는다. 다중 성분 값은 raw만 저장한다.
- 용량이 확인되면 `unit_volume_ml`은 SKU 식별 요소에 반드시 포함하고 `package_count`는 제외한다.
- LOT·제조번호와 RCNO는 `sku_candidate_key_sha256`에 포함하지 않는다.
- 키 구성 요소가 미상이면 확인된 값과 동일하다고 간주하지 않는다. 이때는 `sku_candidate_key_sha256`을 생성하지 않아 확정된 SKU와 자동으로 묶이지 않게 한다.
- 키 구성 요소 미상 자체는 `REVIEW_REQUIRED` 사유가 아니다. 원장 다수가 용량 미표기이며, 이를 검토로 올리면 실제 충돌 사례가 묻힌다. 미상 사실은 정보성 사유 코드로만 남긴다.
- `REVIEW_REQUIRED`는 값 사이의 충돌 또는 문맥 모호성이 있을 때만 사용한다. 값의 부재는 충돌이 아니다.
- 같은 `sku_candidate_key_sha256`은 동일 제품 확정이 아니라 검토 후보를 의미한다.

#### 정제 상태 컬럼

- `normalization_reasons`와 `unparsed_fragments_json`은 빈 배열 `[]`을 기본 형태로 사용한다.
- `normalization_status`는 `PENDING`, `NORMALIZED`, `PARTIAL`, `REVIEW_REQUIRED`, `UNPARSED`, `STALE`을 사용하며 기본값은 `PENDING`이다.
- 원본 의미 해시가 바뀌면 기존 정제값을 신뢰하지 않고 `STALE`로 변경한다.
- 정제 재실행 후 `normalization_version`과 `normalized_at`을 함께 갱신한다.

#### 정제 작업 점유 컬럼

- `claim_owner`와 `claim_lease_until`은 `PENDING`·`STALE` 행을 batch가 점유한 소유자와 lease 만료 시각이다.
- `claim_attempts`와 `claim_next_attempt_at`은 시스템 오류 재시도 횟수와 다음 시도 가능 시각이며 `claim_last_error`에 마지막 원인을 남긴다.
- 점유 컬럼은 작업 제어용이므로 정제 결과 값의 신뢰도를 나타내지 않는다.

#### 리뷰 컬럼

- `review_status`는 `NOT_REQUIRED`, `PENDING`, `APPROVED`, `REJECTED`를 사용한다.
- 정제 결과가 `REVIEW_REQUIRED`이면 `review_status`를 `PENDING`으로 변경한다.
- 리뷰는 정제값을 덮어쓸 수 있지만 `source_item_id`와 `mfds_items` 원본은 수정하지 않는다.
- 사람이 수정한 값은 `reviewed_by`, `reviewed_at`, `review_note`로 근거를 남긴다.

### 8.4 생성과 갱신 규칙

초기 생성은 `mfds_items`에서 RCNO별 최신 1행을 선택하여 수행한다.

```sql
ROW_NUMBER() OVER (
    PARTITION BY rcno
    ORDER BY observed_at DESC, id DESC
) = 1
```

신규 관찰 처리 규칙은 다음과 같다.

1. RCNO가 없으면 최신 `mfds_items.id`를 `source_item_id`로 지정하고 `UNPARSED`로 생성한다.
2. RCNO가 있고 의미 해시가 같으면 `source_item_id`만 최신 관찰 행으로 갱신한다.
3. RCNO가 있고 의미 해시가 다르면 `source_item_id`를 최신 관찰 행으로 바꾸고 `normalization_status = 'STALE'`로 변경한다.
4. 정제 작업은 `UNPARSED` 또는 `STALE` 행을 처리한다.
5. 자동 확정할 수 없는 토큰은 원문을 유지하고 `REVIEW_REQUIRED`, `review_status = 'PENDING'`, 사유를 기록한다.
6. 어떤 경우에도 `mfds_items` 이력을 삭제하거나 갱신하지 않는다.

`mfds-normalization-v3` 전환 data migration은 `mfds-normalization-v2` 결과만 `STALE`로 바꾸고 claim 필드를 초기화한다. v2가 해외제조업소명에서 만든 증류소 후보는 `NULL`로 정리한다. DDL과 data backfill은 별도 migration으로 실행하며 둘 다 재실행 시 같은 결과를 유지한다. `00007` Down은 비가역 data update의 실행 성공만 보장하는 no-op이며, 성공 rollback이 덮어쓴 v2 상태·claim·증류소 후보를 원상복구한다는 뜻이 아니다.

원본 참조와 정제 갱신은 한 트랜잭션에서 수행한다. 정제 도중 실패하면 이전 정제값을 유지하고 상태와 오류 근거를 남긴다.

### 8.5 의도적으로 두지 않는 제약

- FK: 원장 이력 재구성과 개발 편의성을 위해 두지 않는다.
- `sku_candidate_key_sha256` unique: 동일 SKU 후보에 여러 RCNO가 연결될 수 있다.
- 도수·용량 범위 CHECK: 새 형식을 거부하지 않고 검토 대상으로 받아야 한다.
- 회사명·해외제조업소명 unique: 이름 유사성만으로 동일 업체를 확정할 수 없다.

## 9. 자동 처리와 검토 경계

| 구분 | 대상 |
|---|---|
| 자동 처리 | 단위 대소문자, 천 단위 쉼표, 소수 리터, 명시적 묶음 수량 분리, 강한 도수 문맥, 숙성 표기 통일, 명시 LOT, 소비기한 4개 형태 |
| 조건부 구조화 | `SINGLE CASK/CASK`의 숫자 마커, 숫자 `BATCH n`, 제한된 에디션 qualifier, 실제 도수 앵커, 반복 검증된 자재코드, 영문이 같은 음차 표기 차이 |
| 검토 | 이름 중간의 앵커 없는 `%`, `PROOF`, 단독 `CS`, `UNKNOWN` 숫자 마커, 복합 괄호, 빈티지 후보, 언어 간 도수·배치·에디션 값 충돌, 다중 성분 퍼센트, `구형` 대응 후보 |
| 제품키 제외 | RCNO, 명시 LOT·제조번호, 패턴이 확인된 접미 `L` 코드, 박스 입수량, 제조국·수출국 |
| 변형 금지 | 원문 HTML, 제조국·수출국 병합, 의미 특수문자 일괄 제거, 성분 `%`의 도수 변환, `구형↔OLD` 치환, `NEW/LEGACY/RESERVE/CLASSIC` 일괄 제거, 정규화 키 자동 병합 |

### 9.1 BottleNote 제품 상세 호환 후보

1차 정제는 BottleNote `AlcoholDetailItem`과 유사한 형태를 만들되, 원장만으로 확정할 수 없는 값은 후보로 구분한다.

| 구분 | 정제 컬럼 | 근거 |
|---|---|---|
| 직접 정제 | `alcohol_name_ko`, `alcohol_name_en` | 원문 제품명에서 용량·도수·LOT·제조번호만 분리 |
| 직접 정제 | `alcohol_category_ko`, `alcohol_category_en` | 식약처 품목명 4종의 고정 매핑 |
| 직접 정제 | `alcohol_region_ko`, `alcohol_region_en` | 제조국을 지역 1단계 후보로 사용 |
| 직접 정제 | `alcohol_abv` | 명시적으로 확정된 `abv_percent`를 `%` 문자열로 변환 |
| 후보 | `cask_candidate` | 제품명에 캐스크 종류가 명시된 경우에만 추출 |
| 후보 | `distillery_name_ko_candidate`, `distillery_name_en_candidate` | 현재 자동 생성하지 않으며 향후 `distilleries` 사전과 제품명을 대조할 때만 사용 |

- 제조국과 수출국은 각각 한글명, 영문명, ISO 3166-1 Alpha-2, Alpha-3를 저장한다.
- 현재 원장에 존재하는 61개 국가명을 정적 카탈로그로 관리한다.
- 제조국은 BottleNote 지역 후보에 사용하지만 수출국은 지역 후보에 사용하지 않는다.
- 매핑되지 않은 국가나 품목은 추정하지 않고 `REVIEW_REQUIRED` 사유를 기록한다.
- `cask_number`는 캐스크 번호이고 `cask_candidate`는 캐스크 종류이므로 서로 대체하지 않는다.
- 해외제조업소명은 증류소가 아닐 수 있으므로 제품명 근거나 사전 조회 없이 증류소 후보로 복사하지 않는다.

## 10. 변경과 검증 원칙

- 규칙 변경 시 `normalization_version`을 올린다.
- 변경된 규칙은 기존 원문을 기준으로 다시 실행할 수 있어야 한다.
- 규칙 추가 전 실제 데이터 예시와 충돌 사례를 확인한다.
- 원문 손실, 자동 병합, 근거 없는 값 보정은 허용하지 않는다.
- 자동 규칙마다 정상·오탐·경계 fixture를 둔다.
- 규칙 적용 전후 RCNO 수는 변하지 않아야 한다.
- SKU 후보 수 변화와 `review_required` 비율을 버전별로 기록한다.
- 새 특수문자·단위·도수 패턴은 자동 처리하기 전에 실제 원장 예시를 수집한다.

필수 회귀 사례는 다음과 같다.

```text
4,000ml                         -> 4000ml, 000ml 금지
250ML*2                         -> 250ml × 2, 병 SKU 키에는 250ml만 사용
설원 인삼송이주(인삼0.45%)     -> 0.45%를 ABV로 사용 금지
인삼0.45%, 향료0.02%           -> ingredient raw만 저장, 검토
주도38% / (40%,700ml) / 40%VOL -> 실제 도수 앵커로 ABV 저장
LOW LIQUOR 42%                  -> ABV 42 후보
KILCHOMAN 100% ISLAY            -> ABV 사용 금지
가쿠빈 1920ML                   -> vintage_year 사용 금지
몽키숄더 LOT NO. L ...          -> SKU 유지, LOT 분리
발베니 제조번호 ... 2006        -> LOT 제거 전 vintage 추출 금지
글렌알라키 15년 구형            -> version_marker 보존, OLD와 연결 금지
OLD FORESTER                    -> OLD 제거 금지
NEW RIFF                        -> NEW 제거 금지
싱글 몰트 ... / 135066GX0700615 -> 캐스크·자재코드 유지
조니워커 700ml (778061) L5293   -> 자재코드 유지, L코드 LOT 분리
싱글캐스크 (#9485)              -> cask_number 유지
위스키 #77 / @001               -> UNKNOWN/SERIES_NUMBER 원문과 후보 키 유지
CS / CASK STRENGHT              -> 단독 CS 검토, 관측 오타는 CASK STRENGTH alias
SMALL BATCH / BATCH 3           -> 전자는 이름 유지, 후자는 숫자 배치로 구조화
카발란 동일명 51.6% / 57.1%     -> 다른 SKU 후보
고량주 250ml × 20 / × 36        -> 같은 병 SKU 후보, 입수량 제외
스트렝스 / 스트랭스             -> 영문 동일할 때만 검색키 통일
(서울에디션)                    -> 제품명에서 제거 금지
2025-06-03 ~                    -> start 설정, end NULL
~ 2026-08-26                    -> start NULL, end 설정
```
