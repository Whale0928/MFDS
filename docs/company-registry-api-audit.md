# 식품안전나라 수입업체 원장 API 조사

조사 시점은 2026-08-09 KST이며 기존 MySQL 원장은 SELECT만 수행했다. API 응답은 조사 중 변할 수 있는 스냅샷이다.

## 1. 실제 호출 성공 여부

| 서비스 | HTTPS | HTTP | 판정 |
|---|---:|---:|---|
| C001 | 성공 | 성공 | 구현은 HTTPS만 허용 |
| I2821 | 성공 | 성공 | `total_count` 의미 결함 확인 |
| I0250 | 성공 | 성공 | 전체 59행 확인 |
| I0470 | 성공 | 성공 | 전체 5,444행 확인 |

동일 인증키 요청이 겹칠 때 HTTP 200 `text/html` 149 bytes와 "현재 접속 중인 인증키" 안내를 관측했다. 잘못된 인증키도 JSON이 아닌 HTTP 200 HTML을 반환할 수 있어 상태 코드만으로 성공을 판정하지 않는다.

## 2. 전체 건수와 페이지 구조

정상 응답은 `{SERVICE_ID:{total_count:"string",row:[],RESULT:{MSG,CODE}}}` 구조다. C001 81,805행, I0250 59행, I0470 5,444행을 끝까지 조회했다. I2821은 100페이지·100,000행까지 모든 페이지가 1,000행이어서 전체 건수를 확정하지 못했다. I2821 `total_count`는 요청 범위가 1, 2, 5일 때 각각 1, 2, 5를 반환하므로 종료 조건으로 사용하지 않는다.

공식 문서상 요청당 최대 1,000행이고 서비스별 호출 제한은 500이다. 구현은 문서 제한을 우선해 page size 1,000, run당 요청 500회, 서비스별 500페이지로 제한한다. 식품안전나라 메인에는 2026-07-08 게시된 제한적 운영 공지가 있으며 공지상 운영 시간은 09:00~19:00이다. 조사에서는 23시대에도 응답을 받았으므로 공지와 실제 가용 시간의 일치 여부는 미확정이다.

## 3. 실제 응답 필드

- C001 8개: `PRSDNT_NM`, `PRMS_DT`, `LCNS_NO`, `INSTT_NM`, `BSSH_NM`, `LOCP_ADDR`, `TELNO`, `INDUTY_NM`
- I2821 9개: `CLSBIZ_DT`, `PRSDNT_NM`, `PRMS_DT`, `LCNS_NO`, `INSTT_NM`, `BSSH_NM`, `CLSBIZ_DVS_CD_NM`, `LOCP_ADDR`, `INDUTY_NM`
- I0250 9개: `EXCOURY_NATN_CD_NM`, `INCM_PRDT_XPORT_MC_NM`, `PRMS_DT`, `PRDLST_CNT`, `LCNS_NO`, `PRDLST_NM`, `EXCLNC_INCM_BSSH_REGNO`, `BSSH_NM`, `ADDR`
- I0470 17개: `PRSDNT_NM`, `LAST_UPDT_DTM`, `LCNS_NO`, `DSPS_INSTTCD_NM`, `LAWORD_CD_NM`, `DSPSDTLS_SEQ`, `VILTCN`, `ADDR`, `PUBLIC_DT`, `INDUTY_CD_NM`, `DSPS_DCSNDT`, `PRCSCITYPOINT_BSSHNM`, `DSPS_BGNDT`, `DSPS_TYPECD_NM`, `DSPS_ENDDT`, `TELNO`, `DSPSCN`

날짜는 `YYYYMMDD`, `-`, `18991230`, `YYYY-MM-DD HH:MM:SS.fraction`이 혼재한다. raw 원장에서는 전부 문자열 원문으로 보존한다.

## 4. LCNS_NO와 source key 유일성

| 데이터 | 조사 행 | 식별값 | 고유값 | 중복값 수 |
|---|---:|---|---:|---:|
| C001 전체 | 81,805 | `LCNS_NO` | 81,805 | 0 |
| I2821 부분 | 100,000 | `LCNS_NO` | 100,000 | 0 |
| I0250 전체 | 59 | `LCNS_NO` | 40 | 9 |
| I0470 전체 | 5,444 | `DSPSDTLS_SEQ` | 5,428 | 16 |

C001 스냅샷에서는 `LCNS_NO`가 유일했지만 API 계약상 불변 unique key로 확정하지 않는다. I0250은 하나의 인허가에 여러 제품 행이 있고, I0470 `DSPSDTLS_SEQ`도 실제 중복되어 단독 UNIQUE 제약을 두지 않는다.

## 5. C001 결합 결과

| 대상 | 대상 고유 LCNS_NO | C001 결합 고유 LCNS_NO | 결합 행 |
|---|---:|---:|---:|
| I2821 부분 100,000행 | 100,000 | 0 | 0 |
| I0250 전체 | 40 | 38 | 57/59 |
| I0470 전체 | 5,388 | 20 | 20/5,444 |

I2821 미결합은 폐업 원장과 C001 스냅샷의 범위 차이를 보여주는 관측값이며, `I2821에 없음 = 영업 중` 또는 `C001에 없음 = 잘못된 폐업`으로 해석하지 않는다. I0470은 전체 식품업소 처분을 포함하므로 낮은 결합률이 예상되는 구조다.

## 6. 기존 수입업체명 매칭률

기존 `items`에서 RCNO별 최신 12,095행과 고유 수입업체명 396개를 C001 전체와 비교했다.

| 모집단 | EXACT_NAME | NORMALIZED_NAME | AMBIGUOUS | UNRESOLVED | 자동 확정률 |
|---|---:|---:|---:|---:|---:|
| RCNO별 최신 12,095행 | 11,342 | 5 | 612 | 136 | 93.82% |
| 고유 업체명 396개 | 365 | 3 | 23 | 5 | 92.93% |

## 7. 자동 매칭과 모호성

공백만 정리한 이름에서 C001 후보가 정확히 1개면 `EXACT_NAME`, 법인 표기 `주식회사`, `(주)`, `㈜`, `유한회사`와 공백·괄호를 제거한 이름에서 후보가 정확히 1개면 `NORMALIZED_NAME`으로 저장한다. 후보가 2개 이상이면 `AMBIGUOUS`, 없으면 `UNRESOLVED`로 보존하고 자동 병합하지 않는다.

C001에서 같은 `BSSH_NM`이 복수 `LCNS_NO`를 가진 이름은 4,259개였고 한 이름의 최대 인허가 수는 25개였다. 따라서 회사 엔티티와 영업소 인허가 엔티티는 분리해야 한다. 주소를 포함한 자동 확정은 주소 이력과 본점·영업소 의미를 검증하기 전까지 적용하지 않는다.

## 8. raw 테이블과 인덱스

- `company_registry_runs`: 실행 상태와 비밀정보를 제외한 설정 snapshot
- `company_registry_fetches`: 마스킹 경로, 요청 범위, 응답 header, gzip 원문, hash, RESULT와 오류
- `c001_importer_licenses_raw`: `license_no`, 업소명·검색키, 업종 인덱스
- `i2821_importer_closures_raw`: `license_no`, 업소명, 폐업일·상태 인덱스
- `i0250_excellent_importers_raw`: 등록번호, `license_no`, 업소명, 제조사명 인덱스
- `i0470_administrative_dispositions_raw`: 전산키, `license_no`, 확정일, 처분유형 인덱스
- `importer_license_match_evidence`: RCNO, 상태, 선택 LCNS_NO, matcher version 인덱스

상태는 모두 MySQL ENUM이 아닌 `VARCHAR`와 Go 상수로 관리한다. 모든 컬럼과 테이블에 한글 `COMMENT`를 둔다.

## 9. 기존 수집기 변경 범위

기존 `collect`, `normalize`, `items`, `declarations`의 수집·정제 로직은 변경하지 않는다. 새 source client, usecase, MySQL store와 `collect-company-registry` Cobra 명령만 추가한다. 기존 `items`는 매칭 후보 조회에만 사용하고 수정하지 않는다.

## 10. 미확정 사항

- I2821의 실제 전체 건수와 마지막 페이지
- C001/I2821/I0470 필터별 선택도와 변경일자의 포함 경계
- C001에서 누락된 폐업 인허가를 포함하는 영업 상태 산정 규칙
- I0250 Open API와 문서가 안내하는 월간 PDF 사이의 최신성 차이
- `PUBLIC_DT`가 지난 행의 서비스 화면 노출 정책
- 주소·상호 이력을 반영한 `NAME_AND_ADDRESS`, `CONFIRMED_ALIAS`, `MANUAL` 확정 절차

## 공식 문서

- [Open API 이용방법](https://www.foodsafetykorea.go.kr/api/howToUseApi.do)
- [C001](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C001)
- [I2821](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I2821)
- [I0250](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I0250)
- [I0470](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I0470)
- [Open API 공지](https://www.foodsafetykorea.go.kr/api/main.do)
