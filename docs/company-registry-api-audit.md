# 식품안전나라 수입업체 원장 API 조사

조사 시점은 2026-08-10 KST이며 기존 MySQL 원장은 SELECT만 수행했다. API 응답은 조사 중 변할 수 있는 스냅샷이다.

## 1. 실제 호출 성공 여부

| 서비스 | HTTPS | HTTP | 판정 |
|---|---:|---:|---|
| 수입식품 영업신고 정보(C001) | 성공 | 성공 | 구현은 HTTPS만 허용 |
| 수입식품업 폐업정보(I2821) | 성공 | 성공 | `total_count` 의미 결함 확인 |
| 우수수입업소 현황(I0250) | 성공 | 성공 | 전체 59행 확인 |
| 행정처분 결과(I0470) | 성공 | 성공 | 페이지 이동 중 전체 건수 변동 확인 |

동일 인증키 요청이 겹칠 때 HTTP 200 `text/html` 149 bytes와 "현재 접속 중인 인증키" 안내를 관측했다. 잘못된 인증키도 JSON이 아닌 HTTP 200 HTML을 반환할 수 있어 상태 코드만으로 성공을 판정하지 않는다.

## 2. 전체 건수와 페이지 구조

정상 응답은 `{SERVICE_ID:{total_count:"string",row:[],RESULT:{MSG,CODE}}}` 구조다. 2026-08-10 00:42 KST 재확인 값은 C001 81,805행, I0250 59행, I0470 5,451행이다. I2821은 마지막 존재 인덱스 159,364와 빈 응답 인덱스 159,365를 실제 호출로 확인했으므로 관측 전체는 159,364행이다. 다만 I2821 `total_count`는 요청 범위가 1, 2, 5일 때 각각 1, 2, 5를 반환하므로 전체 건수 필드로 사용할 수 없다.

전체 동기화는 1,000행 단위로 C001 82회, I2821 160회, I0250 1회, I0470 6회인 총 249회 요청이다. 0.5 QPS 설정에서는 약 8~10분이 필요하다.

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
| I0470 관측 | 5,444 | `DSPSDTLS_SEQ` | 5,428 | 16 |

C001 스냅샷에서는 `LCNS_NO`가 유일했지만 API 계약상 불변 unique key로 확정하지 않는다. I0250은 하나의 인허가에 여러 제품 행이 있고, I0470 `DSPSDTLS_SEQ`도 실제 중복되어 단독 UNIQUE 제약을 두지 않는다.

## 5. 수입식품 영업신고 정보(C001) 결합 조사 결과

| 대상 | 대상 고유 LCNS_NO | C001 결합 고유 LCNS_NO | 결합 행 |
|---|---:|---:|---:|
| I2821 부분 100,000행 | 100,000 | 0 | 0 |
| I0250 전체 | 40 | 38 | 57/59 |
| I0470 관측 | 5,388 | 20 | 20/5,444 |

I2821 미결합은 폐업 원장과 C001 스냅샷의 범위 차이를 보여주는 관측값이며, `I2821에 없음 = 영업 중` 또는 `C001에 없음 = 잘못된 폐업`으로 해석하지 않는다. I0470은 전체 식품업소 처분을 포함하므로 낮은 결합률이 예상되는 구조다.

## 6. 기존 수입업체명 비교 조사 결과

기존 `items`에서 RCNO별 최신 12,095행과 고유 수입업체명 396개를 C001 전체와 비교했다.

| 모집단 | EXACT_NAME | AMBIGUOUS | UNRESOLVED | 자동 확정률 |
|---|---:|---:|---:|---:|
| RCNO별 최신 12,095행 | 11,351 | 603 | 141 | 93.85% |
| 고유 업체명 396개 | 367 | 21 | 8 | 92.68% |

## 7. 동기화 필터와 조회 시점 비교

공식 API에는 업종 필터가 없다. 대신 수입식품 영업신고 정보(C001), 수입식품업 폐업정보(I2821), 행정처분 결과(I0470)는 공식 변경일자(`CHNG_DT`) 필터를 사용한다. 최초 실행은 사용자가 `--since YYYY-MM-DD`를 지정하고, 이후 실행은 마지막 완료 실행의 기준일을 다시 포함해 변경분을 저장한다. 우수수입업소 현황(I0250)은 공식 필터가 없어 매번 전체를 조회한다.

연결 상태나 매칭 근거는 저장하지 않는다. 대시보드에서 수입 기록 상세를 열 때 기존 원장의 업체명과 수입식품 영업신고 정보(C001) `BSSH_NM` 문자열을 대소문자까지 그대로 비교해 현재 원장을 조회한다. 같은 이름의 인허가가 여러 개면 하나로 합치지 않고 모두 보여주며, 법인 표기와 공백이 다른 이름은 자동으로 같은 업체로 간주하지 않는다.

비교 대상 업종에서 같은 `BSSH_NM`이 복수 `LCNS_NO`를 가진 이름은 1,319개였고 한 이름의 최대 인허가 수는 13개였다. 따라서 회사 엔티티와 영업소 인허가 엔티티는 분리해야 한다. 주소를 포함한 자동 확정은 주소 이력과 본점·영업소 의미를 검증하기 전까지 적용하지 않는다.

## 8. raw 테이블과 인덱스

- `company_registry_runs`: 실행 상태와 비밀정보를 제외한 설정 snapshot
- `company_registry_fetches`: 마스킹 경로, 요청 범위, 응답 header, gzip 원문, hash, RESULT와 오류
- `c001_importer_licenses_raw`: `license_no`, 업소명, 업종 인덱스
- `i2821_importer_closures_raw`: `license_no`, 업소명, 폐업일·상태 인덱스
- `i0250_excellent_importers_raw`: 등록번호, `license_no`, 업소명, 제조사명 인덱스
- `i0470_administrative_dispositions_raw`: 전산키, `license_no`, 확정일, 처분유형 인덱스

상태는 모두 MySQL ENUM이 아닌 `VARCHAR`와 Go 상수로 관리한다. 모든 컬럼과 테이블에 한글 `COMMENT`를 둔다.

## 9. 기존 수집기 변경 범위

기존 `collect`, `normalize`, `items`, `declarations`의 수집·정제 로직은 변경하지 않는다. 새 출처 클라이언트(source client), 사용 사례(usecase), MySQL 저장소와 `sync-company-registry` 코브라(Cobra) 명령을 별도로 추가한다. 동기화 명령은 기존 `items`를 조회하거나 수정하지 않는다. 대시보드의 수입 기록 상세 API만 현재 행의 수입사명으로 공식정보 원장을 즉석 조회한다.

## 10. 미확정 사항

- I0470의 일관된 시점 전체 건수와 pagination 중 데이터 변동 처리
- C001/I2821/I0470 필터별 선택도와 변경일자의 포함 경계
- C001에서 누락된 폐업 인허가를 포함하는 영업 상태 산정 규칙
- I0250 Open API와 문서가 안내하는 월간 PDF 사이의 최신성 차이
- 공개기한(`PUBLIC_DT`, 행정처분 정보를 공개할 수 있는 기한)이 지난 행의 서비스 화면 노출 정책
- 주소·상호 이력을 반영한 `NAME_AND_ADDRESS`, `CONFIRMED_ALIAS`, `MANUAL` 확정 절차

## 공식 문서

- [Open API 이용방법](https://www.foodsafetykorea.go.kr/api/howToUseApi.do)
- [C001](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C001)
- [I2821](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I2821)
- [I0250](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I0250)
- [I0470](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I0470)
- [Open API 공지](https://www.foodsafetykorea.go.kr/api/main.do)
