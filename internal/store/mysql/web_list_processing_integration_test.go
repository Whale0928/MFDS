package mysql

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdsweb"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

var integrationTargets = []weblist.Target{
	{Name: "위스키", Code: "C0314210000000000000"},
	{Name: "브랜디", Code: "C0314220000000000000"},
	{Name: "일반증류주", Code: "C0314230000000000000"},
	{Name: "리큐르", Code: "C0314240000000000000"},
}

func integrationStoreV2(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MFDS_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("MFDS_INTEGRATION_DSN이 설정되지 않았습니다")
	}
	store, err := Open(config.DatabaseConfig{
		DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 4,
		ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func integrationServiceV2(t *testing.T, store *Store, serverURL string) *weblist.Service {
	t.Helper()
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mfdsweb.NewClient(mfdsweb.ClientOptions{BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	service, err := weblist.NewService(store, mfdsweb.NewUsecaseAdapter(client), weblist.Options{
		Targets: integrationTargets, PageSize: 2, QPS: 0, MaxAttempts: 3,
		RetryDelays: []time.Duration{time.Millisecond}, Location: location,
		WebBaseURL: serverURL, ProcessID: "0011223344556677",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func cleanupIntegrationJob(t *testing.T, store *Store, jobID uint64) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := store.db.Exec("DELETE FROM jobs WHERE id = ?", jobID); err != nil {
			t.Errorf("integration Job cleanup error = %v", err)
		}
	})
}

func TestWebListProcessingIntegration_날짜Task_4개품목과추가페이지를저장한다(t *testing.T) {
	page1 := readFixture(t, "../../source/mfdsweb/testdata/list_page1.html")
	page2 := readFixture(t, "../../source/mfdsweb/testdata/list_page2.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF-8")
		if request.URL.Query().Get("page") == "2" {
			_, _ = writer.Write(adaptFixture(page2, request))
			return
		}
		_, _ = writer.Write(adaptFixture(page1, request))
	}))
	defer server.Close()

	store := integrationStoreV2(t)
	service := integrationServiceV2(t, store, server.URL)
	result, err := service.ExecuteJob(context.Background(), weblist.JobCommand{
		FromDate: "2026-07-27", ToDate: "2026-07-27", Workers: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	cleanupIntegrationJob(t, store, result.RunID)
	if result.Status != weblist.RunStatusCompleted ||
		result.TotalPartitions != 1 || result.CompletedPartitions != 1 {
		t.Fatalf("result = %+v", result)
	}
	var taskCount, itemKinds, fetchCount, itemCount, attempts, acceptedRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE job_id=?", result.RunID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(DISTINCT item_code) FROM fetches WHERE job_id=?", result.RunID).Scan(&itemKinds); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM fetches WHERE job_id=?", result.RunID).Scan(&fetchCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM items WHERE job_id=?", result.RunID).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(
		"SELECT attempts, parsed_rows FROM tasks WHERE job_id=?",
		result.RunID,
	).Scan(&attempts, &acceptedRows); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || itemKinds != 4 || fetchCount != 16 || itemCount != 32 ||
		attempts != 2 || acceptedRows != 16 || result.ParsedRows != 16 {
		t.Fatalf(
			"tasks=%d itemKinds=%d fetches=%d items=%d attempts=%d acceptedRows=%d resultRows=%d",
			taskCount,
			itemKinds,
			fetchCount,
			itemCount,
			attempts,
			acceptedRows,
			result.ParsedRows,
		)
	}
}

func TestWebListProcessingIntegration_페이지경계RCNO중복_날짜전체를재검증한다(t *testing.T) {
	page1 := readFixture(t, "../../source/mfdsweb/testdata/list_page1.html")
	page2 := readFixture(t, "../../source/mfdsweb/testdata/list_page2.html")
	var corrupted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF-8")
		response := page1
		if request.URL.Query().Get("page") == "2" {
			response = page2
			if request.URL.Query().Get("rpsntItmCd") == integrationTargets[0].Code &&
				corrupted.CompareAndSwap(false, true) {
				response = bytes.ReplaceAll(
					response,
					[]byte("202600522066"),
					[]byte("202600524242"),
				)
			}
		}
		_, _ = writer.Write(adaptFixture(response, request))
	}))
	defer server.Close()

	store := integrationStoreV2(t)
	service := integrationServiceV2(t, store, server.URL)
	result, err := service.ExecuteJob(context.Background(), weblist.JobCommand{
		FromDate: "2026-07-23", ToDate: "2026-07-23", Workers: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	cleanupIntegrationJob(t, store, result.RunID)

	var attempts, acceptedRows, acceptedUnique, rawRows int
	if err := store.db.QueryRow(`
		SELECT attempts, parsed_rows, unique_rcno_count
		FROM tasks WHERE job_id=?
	`, result.RunID).Scan(&attempts, &acceptedRows, &acceptedUnique); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(
		"SELECT COUNT(*) FROM items WHERE job_id=?",
		result.RunID,
	).Scan(&rawRows); err != nil {
		t.Fatal(err)
	}
	var firstAttemptUnique, secondAttemptUnique, thirdAttemptUnique int
	for attempt, destination := range map[int]*int{
		1: &firstAttemptUnique,
		2: &secondAttemptUnique,
		3: &thirdAttemptUnique,
	} {
		if err := store.db.QueryRow(`
			SELECT COUNT(DISTINCT i.rcno)
			FROM items AS i
			JOIN fetches AS f ON f.id=i.fetch_id
			WHERE i.job_id=? AND f.item_code=? AND f.attempt_no=?
		`, result.RunID, integrationTargets[0].Code, attempt).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	if result.Status != weblist.RunStatusCompleted ||
		attempts != 3 || acceptedRows != 16 || acceptedUnique != 4 || rawRows != 48 ||
		firstAttemptUnique != 3 || secondAttemptUnique != 4 || thirdAttemptUnique != 4 {
		t.Fatalf(
			"status=%s attempts=%d acceptedRows=%d acceptedUnique=%d rawRows=%d attemptUnique=%d/%d/%d",
			result.Status,
			attempts,
			acceptedRows,
			acceptedUnique,
			rawRows,
			firstAttemptUnique,
			secondAttemptUnique,
			thirdAttemptUnique,
		)
	}
}

func TestWebListProcessingIntegration_부분실패후Task전체재시도_중복키를위반하지않는다(t *testing.T) {
	page1 := readFixture(t, "../../source/mfdsweb/testdata/list_page1.html")
	page2 := readFixture(t, "../../source/mfdsweb/testdata/list_page2.html")
	var failed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("rpsntItmCd") == integrationTargets[1].Code &&
			request.URL.Query().Get("page") == "1" && failed.CompareAndSwap(false, true) {
			http.Error(writer, "temporary", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=UTF-8")
		if request.URL.Query().Get("page") == "2" {
			_, _ = writer.Write(adaptFixture(page2, request))
			return
		}
		_, _ = writer.Write(adaptFixture(page1, request))
	}))
	defer server.Close()

	store := integrationStoreV2(t)
	service := integrationServiceV2(t, store, server.URL)
	result, err := service.ExecuteJob(context.Background(), weblist.JobCommand{
		FromDate: "2026-07-26", ToDate: "2026-07-26", Workers: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	cleanupIntegrationJob(t, store, result.RunID)
	var attempts, failedFetches, duplicateKeys int
	if err := store.db.QueryRow("SELECT attempts FROM tasks WHERE job_id=?", result.RunID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM fetches WHERE job_id=? AND status='FAILED'", result.RunID).Scan(&failedFetches); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT job_id, request_key_sha256, attempt_no, COUNT(*) count
			FROM fetches WHERE job_id=?
			GROUP BY job_id, request_key_sha256, attempt_no HAVING count > 1
		) duplicated
	`, result.RunID).Scan(&duplicateKeys); err != nil {
		t.Fatal(err)
	}
	if result.Status != weblist.RunStatusCompleted ||
		attempts != 3 || failedFetches != 1 || duplicateKeys != 0 {
		t.Fatalf("status=%s attempts=%d failedFetches=%d duplicateKeys=%d",
			result.Status, attempts, failedFetches, duplicateKeys)
	}
}

func TestWebListProcessingIntegration_빈결과날짜_4개Fetch로완료한다(t *testing.T) {
	empty := readFixture(t, "../../source/mfdsweb/testdata/list_empty.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = writer.Write(adaptFixture(empty, request))
	}))
	defer server.Close()

	store := integrationStoreV2(t)
	service := integrationServiceV2(t, store, server.URL)
	result, err := service.ExecuteJob(context.Background(), weblist.JobCommand{
		FromDate: "2026-07-25", ToDate: "2026-07-25", Workers: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	cleanupIntegrationJob(t, store, result.RunID)
	var fetches, items int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM fetches WHERE job_id=?", result.RunID).Scan(&fetches); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM items WHERE job_id=?", result.RunID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if result.Status != weblist.RunStatusCompleted || fetches != 4 || items != 0 {
		t.Fatalf("status=%s fetches=%d items=%d", result.Status, fetches, items)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture %s read error = %v", path, err)
	}
	return value
}

func adaptFixture(fixture []byte, request *http.Request) []byte {
	value := bytes.ReplaceAll(fixture, []byte("위스키"), []byte(request.URL.Query().Get("rpsntItmNm")))
	value = bytes.ReplaceAll(value, []byte("브랜디"), []byte(request.URL.Query().Get("rpsntItmNm")))
	value = bytes.ReplaceAll(value, []byte("C0314210000000000000"), []byte(request.URL.Query().Get("rpsntItmCd")))
	value = bytes.ReplaceAll(value, []byte("C0314220000000000000"), []byte(request.URL.Query().Get("rpsntItmCd")))
	value = bytes.ReplaceAll(value, []byte("2026-07-27"), []byte(request.URL.Query().Get("srchStrtDt")))
	value = bytes.ReplaceAll(value, []byte("2025-07-28"), []byte(request.URL.Query().Get("srchStrtDt")))
	value = bytes.ReplaceAll(value, []byte(`name="limit" value="10"`),
		[]byte(`name="limit" value="`+request.URL.Query().Get("limit")+`"`))
	return value
}

func TestClaimTask_동시실행_같은Task를한번만할당한다(t *testing.T) {
	store := integrationStoreV2(t)
	record, err := store.StartWebListJob(context.Background(), weblist.JobStartParams{
		FromDate:  time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local),
		ToDate:    time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local),
		StartedAt: time.Now(), PageSize: 2, ConfigJSON: []byte(`{"test":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupIntegrationJob(t, store, record.RunID)

	results := make(chan bool, 2)
	for index := range 2 {
		go func(worker int) {
			_, found, claimErr := store.ClaimTask(context.Background(), weblist.ClaimParams{
				RunID: record.RunID, Owner: fmt.Sprintf("worker-%d", worker),
				Now: time.Now(), LeaseUntil: time.Now().Add(time.Minute),
			})
			if claimErr != nil {
				t.Errorf("ClaimTask() error = %v", claimErr)
			}
			results <- found
		}(index)
	}
	foundCount := 0
	for range 2 {
		if <-results {
			foundCount++
		}
	}
	if foundCount != 1 {
		t.Fatalf("found count = %d, want 1", foundCount)
	}
}
