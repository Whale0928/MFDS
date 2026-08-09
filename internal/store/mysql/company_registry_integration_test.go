package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

type integrationCompanyRegistryClient struct {
	now time.Time
}

func (client integrationCompanyRegistryClient) FetchPage(
	_ context.Context,
	request companyregistry.PageRequest,
) (companyregistry.Page, error) {
	raw := integrationCompanyRegistryRow(request.Service)
	body := []byte(fmt.Sprintf(`{"%s":{"total_count":"1","row":[%s],"RESULT":{"MSG":"정상","CODE":"INFO-000"}}}`, request.Service, raw))
	return companyregistry.Page{
		Service: request.Service, StartIndex: request.StartIndex, EndIndex: request.EndIndex,
		Attempt: request.Attempt, RequestPathRedacted: fmt.Sprintf("/api/REDACTED/%s/json/%d/%d", request.Service, request.StartIndex, request.EndIndex),
		RequestFilterJSON: json.RawMessage(`{}`), HTTPStatus: 200, ContentType: "application/json",
		ResultCode: "INFO-000", ResultMessage: "정상", TotalCount: 1,
		Rows: []json.RawMessage{raw}, RawBody: body,
		StartedAt: client.now, FinishedAt: client.now.Add(time.Millisecond),
	}, nil
}

func integrationCompanyRegistryRow(service companyregistry.ServiceID) json.RawMessage {
	switch service {
	case companyregistry.ServiceC001:
		return json.RawMessage(`{"PRSDNT_NM":"대표","PRMS_DT":"20200123","LCNS_NO":"L-1","INSTT_NM":"기관","BSSH_NM":"테스트 수입사","LOCP_ADDR":"주소","TELNO":"02","INDUTY_NM":"수입식품등 수입판매업"}`)
	case companyregistry.ServiceI2821:
		return json.RawMessage(`{"CLSBIZ_DT":"20040515","PRSDNT_NM":"대표","PRMS_DT":"18991230","LCNS_NO":"OLD-1","INSTT_NM":"기관","BSSH_NM":"폐업 업체","CLSBIZ_DVS_CD_NM":"폐업","LOCP_ADDR":"주소","INDUTY_NM":"수입판매업"}`)
	case companyregistry.ServiceI0250:
		return json.RawMessage(`{"EXCOURY_NATN_CD_NM":"미국","INCM_PRDT_XPORT_MC_NM":"제조사","PRMS_DT":"20190611","PRDLST_CNT":"1","LCNS_NO":"L-1","PRDLST_NM":"위스키","EXCLNC_INCM_BSSH_REGNO":"E-1","BSSH_NM":"테스트 수입사","ADDR":"주소"}`)
	case companyregistry.ServiceI0470:
		return json.RawMessage(`{"PRSDNT_NM":"대표","LAST_UPDT_DTM":"2024-08-13 18:41:31.649","LCNS_NO":"L-1","DSPS_INSTTCD_NM":"기관","LAWORD_CD_NM":"법령","DSPSDTLS_SEQ":"D-1","VILTCN":"위반","ADDR":"주소","PUBLIC_DT":"2026-08-11 00:00:00.0","INDUTY_CD_NM":"수입판매업","DSPS_DCSNDT":"20240812","PRCSCITYPOINT_BSSHNM":"테스트 수입사","DSPS_BGNDT":"20240812","DSPS_TYPECD_NM":"시정명령","DSPS_ENDDT":"-","TELNO":"02","DSPSCN":"처분"}`)
	default:
		return json.RawMessage(`{}`)
	}
}

func TestCompanyRegistryStore_네서비스Raw와매칭근거를한실행에저장한다(t *testing.T) {
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture.item(t, fmt.Sprintf("REG-%d", now.UnixNano()), "registry", now, now)
	service, err := companyregistry.NewService(store, integrationCompanyRegistryClient{now: now}, companyregistry.Options{
		PageSize: 2, MaxPages: 2, MaxRequests: 10, QPS: 1000, MaxAttempts: 1,
		MatcherVersion: "integration-v1",
	})
	if err != nil {
		t.Fatal(err)
	}

	summary, err := service.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.db.Exec("DELETE FROM importer_license_match_evidence WHERE run_id = ?", summary.CollectionID); cleanupErr != nil {
			t.Errorf("company registry evidence cleanup failed: %v", cleanupErr)
		}
		if _, cleanupErr := store.db.Exec("DELETE FROM company_registry_runs WHERE id = ?", summary.CollectionID); cleanupErr != nil {
			t.Errorf("company registry cleanup failed: %v", cleanupErr)
		}
	})

	counts := make([]int, 0, 7)
	for _, query := range []string{
		"SELECT COUNT(*) FROM company_registry_fetches WHERE run_id = ?",
		"SELECT COUNT(*) FROM c001_importer_licenses_raw r JOIN company_registry_fetches f ON f.id=r.fetch_id WHERE f.run_id = ?",
		"SELECT COUNT(*) FROM i2821_importer_closures_raw r JOIN company_registry_fetches f ON f.id=r.fetch_id WHERE f.run_id = ?",
		"SELECT COUNT(*) FROM i0250_excellent_importers_raw r JOIN company_registry_fetches f ON f.id=r.fetch_id WHERE f.run_id = ?",
		"SELECT COUNT(*) FROM i0470_administrative_dispositions_raw r JOIN company_registry_fetches f ON f.id=r.fetch_id WHERE f.run_id = ?",
		"SELECT COUNT(*) FROM importer_license_match_evidence WHERE run_id = ? AND match_status = 'EXACT_NAME'",
		"SELECT COUNT(*) FROM company_registry_runs WHERE id = ? AND status = 'COMPLETED'",
	} {
		var count int
		if err := store.db.QueryRow(query, summary.CollectionID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts = append(counts, count)
	}
	if fmt.Sprint(counts) != "[4 1 1 1 1 1 1]" || summary.Matches[companyregistry.MatchExactName] != 1 {
		t.Fatalf("counts=%v summary=%+v", counts, summary)
	}
	var tableCount, missingComments, enumColumns int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN (
			'company_registry_runs', 'company_registry_fetches', 'c001_importer_licenses_raw',
			'i2821_importer_closures_raw', 'i0250_excellent_importers_raw',
			'i0470_administrative_dispositions_raw', 'importer_license_match_evidence'
		)
	`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN (
			'company_registry_runs', 'company_registry_fetches', 'c001_importer_licenses_raw',
			'i2821_importer_closures_raw', 'i0250_excellent_importers_raw',
			'i0470_administrative_dispositions_raw', 'importer_license_match_evidence'
		) AND (COLUMN_COMMENT IS NULL OR COLUMN_COMMENT = '')
	`).Scan(&missingComments); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN (
			'company_registry_runs', 'company_registry_fetches', 'c001_importer_licenses_raw',
			'i2821_importer_closures_raw', 'i0250_excellent_importers_raw',
			'i0470_administrative_dispositions_raw', 'importer_license_match_evidence'
		) AND DATA_TYPE = 'enum'
	`).Scan(&enumColumns); err != nil {
		t.Fatal(err)
	}
	if tableCount != 7 || missingComments != 0 || enumColumns != 0 {
		t.Fatalf("tables=%d missing_comments=%d enum_columns=%d", tableCount, missingComments, enumColumns)
	}
}
