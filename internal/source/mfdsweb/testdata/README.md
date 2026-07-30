# MFDS list fixtures

감사 시 저장한 실제 MFDS HTML과 확인된 DOM shape를 보존한 테스트 fixture다.

## 감사 원본

아래 파일은 `/CFCCC01F01/getList`의 익명 GET 응답 원본을 `gzip -n -9`로
결정적으로 압축하고 Base64로 인코딩했다. 테스트는 Base64와 gzip을 역변환한 뒤
원본 byte size와 SHA-256을 먼저 검증하고 parser에 전달한다.

| encoded fixture | redacted query | original bytes | original SHA-256 |
| --- | --- | ---: | --- |
| `audit-order-page1.html.gz.b64` | item=위스키, date=2026-07-27, page=1, limit=10 | 631646 | `ddd5d858b706a83da3074469415ded66748dc40b2145bfbcb15ee1dd631cafa9` |
| `audit-retention-empty.html.gz.b64` | item=브랜디, date=2025-07-28, page=1, limit=1 | 614644 | `9824cf4a47b27c2176894b90f1299a6a5dbc36f53e6de6e88738f02d0d183e27` |
| `audit-invalid-date-error.html.gz.b64` | invalid-date probe, redirected generic error response | 5153 | `26aed11b0cf519f613952316b648cae15cd38d460c7dff539f87d43479a2397c` |

원본 요청의 cookie, session header, API key, DSN, proxy 인증정보는 저장하지 않았다.

## Compact contract fixtures

- `list_page1.html`: 단일 처리일, total 4, page 1/2, limit 2, 정상 row 2개
- `list_page2.html`: 같은 snapshot의 page 2/2와 정상 row 2개
- `list_empty.html`: 단일 처리일 total 0과 `nodata` row
- `error.html`: invalid date redirect 뒤 확인된 generic error shape

운영 사이트를 테스트 중 호출하지 않고 `httptest.Server` 또는 parser fixture로만 사용한다.
