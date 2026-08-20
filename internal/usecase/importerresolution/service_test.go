package importerresolution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
)

type fakeStore struct {
	groups []PendingGroup
	saved  []Resolution
	err    error
}

func (s *fakeStore) ListPendingImporterGroups(context.Context, uint64) ([]PendingGroup, error) {
	return s.groups, s.err
}

func (s *fakeStore) SaveImporterResolutions(_ context.Context, resolutions []Resolution) error {
	s.saved = append(s.saved, resolutions...)
	return s.err
}

type fakeSource struct {
	search        func(mfdscompany.SearchRequest) (mfdscompany.SearchPage, error)
	detail        func(string) (mfdscompany.BusinessDetail, error)
	searchGallery func(mfdscompany.GallerySearchRequest) (mfdscompany.GalleryPage, error)
	galleryDetail func(string) (mfdscompany.GalleryDetail, error)
}

func (s fakeSource) Search(_ context.Context, request mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
	return s.search(request)
}

func (s fakeSource) Detail(_ context.Context, code string) (mfdscompany.BusinessDetail, error) {
	return s.detail(code)
}

func (s fakeSource) SearchGallery(_ context.Context, request mfdscompany.GallerySearchRequest) (mfdscompany.GalleryPage, error) {
	return s.searchGallery(request)
}

func (s fakeSource) GalleryDetail(_ context.Context, productCode string) (mfdscompany.GalleryDetail, error) {
	return s.galleryDetail(productCode)
}

func newTestService(t *testing.T, store Store, source Source) *Service {
	t.Helper()
	service, err := NewService(store, source, Options{
		PageSize: 50,
		Industry: "141",
		State:    "ING",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestService_SyncJob_exact단일후보면그룹의모든RCNO를연결한다(t *testing.T) {
	store := &fakeStore{groups: []PendingGroup{{
		BusinessName: "주식회사 호수무역",
		ProcessDate:  time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		RCNOs:        []string{"RCNO-1", "RCNO-2"},
	}}}
	source := fakeSource{
		search: func(request mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
			if request.BusinessName != "주식회사 호수무역" {
				t.Fatalf("business name = %q", request.BusinessName)
			}
			return mfdscompany.SearchPage{TotalPages: 1, Rows: []mfdscompany.SearchResult{{
				InternalBusinessCode: "BUSINESS-1", BusinessName: request.BusinessName,
			}}, Source: sourceMetadata("list")}, nil
		},
		detail: func(code string) (mfdscompany.BusinessDetail, error) {
			return mfdscompany.BusinessDetail{
				InternalBusinessCode: code, BusinessName: "주식회사 호수무역", Source: sourceMetadata("detail"),
			}, nil
		},
		searchGallery: unexpectedGallerySearch(t),
		galleryDetail: unexpectedGalleryDetail(t),
	}

	summary, err := newTestService(t, store, source).SyncJob(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Resolved != 2 || summary.Unresolved != 0 || len(store.saved) != 2 {
		t.Fatalf("summary=%+v saved=%+v", summary, store.saved)
	}
	for _, resolution := range store.saved {
		if resolution.Importer.InternalBusinessCode != "BUSINESS-1" || resolution.GallerySource != nil {
			t.Fatalf("resolution=%+v", resolution)
		}
	}
}

func TestService_SyncJob_복수후보면갤러리RCNO와업소코드가일치한건만연결한다(t *testing.T) {
	processedDate := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{groups: []PendingGroup{{
		BusinessName: "에이스무역", ProcessDate: processedDate, RCNOs: []string{"RCNO-A", "RCNO-B"},
	}}}
	source := fakeSource{
		search: func(request mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
			return mfdscompany.SearchPage{TotalPages: 1, Rows: []mfdscompany.SearchResult{
				{InternalBusinessCode: "BUSINESS-A", BusinessName: request.BusinessName},
				{InternalBusinessCode: "BUSINESS-B", BusinessName: request.BusinessName},
			}, Source: sourceMetadata("list")}, nil
		},
		detail: func(code string) (mfdscompany.BusinessDetail, error) {
			return mfdscompany.BusinessDetail{InternalBusinessCode: code, BusinessName: "에이스무역", Source: sourceMetadata("detail")}, nil
		},
		searchGallery: func(request mfdscompany.GallerySearchRequest) (mfdscompany.GalleryPage, error) {
			if request.BusinessName != "에이스무역" || !request.FromDate.Equal(processedDate) || !request.ToDate.Equal(processedDate) {
				t.Fatalf("gallery request=%+v", request)
			}
			return mfdscompany.GalleryPage{TotalPages: 1, Products: []mfdscompany.GalleryProduct{{ProductCode: "PRODUCT-1"}}}, nil
		},
		galleryDetail: func(productCode string) (mfdscompany.GalleryDetail, error) {
			return mfdscompany.GalleryDetail{
				ProductCode: productCode, RCNO: "RCNO-B", InternalBusinessCode: "BUSINESS-B",
				BusinessName: "에이스무역", Source: sourceMetadata("gallery"),
			}, nil
		},
	}

	summary, err := newTestService(t, store, source).SyncJob(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Resolved != 1 || summary.Unresolved != 1 || len(store.saved) != 1 {
		t.Fatalf("summary=%+v saved=%+v", summary, store.saved)
	}
	resolution := store.saved[0]
	if resolution.RCNO != "RCNO-B" || resolution.Importer.InternalBusinessCode != "BUSINESS-B" || resolution.GallerySource == nil {
		t.Fatalf("resolution=%+v", resolution)
	}
}

func TestService_SyncJob_후보가없으면상태나연결을저장하지않는다(t *testing.T) {
	store := &fakeStore{groups: []PendingGroup{{BusinessName: "없는 상호", ProcessDate: time.Now(), RCNOs: []string{"RCNO-X"}}}}
	source := fakeSource{
		search: func(mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
			return mfdscompany.SearchPage{TotalPages: 0}, nil
		},
		detail: unexpectedDetail(t), searchGallery: unexpectedGallerySearch(t), galleryDetail: unexpectedGalleryDetail(t),
	}

	summary, err := newTestService(t, store, source).SyncJob(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Resolved != 0 || summary.Unresolved != 1 || len(store.saved) != 0 {
		t.Fatalf("summary=%+v saved=%+v", summary, store.saved)
	}
}

func TestService_SyncJob_복수후보인데처리일이없으면추정하지않는다(t *testing.T) {
	store := &fakeStore{groups: []PendingGroup{{BusinessName: "동명 상호", RCNOs: []string{"RCNO-Z"}}}}
	source := fakeSource{
		search: func(request mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
			return mfdscompany.SearchPage{TotalPages: 1, Rows: []mfdscompany.SearchResult{
				{InternalBusinessCode: "BUSINESS-1", BusinessName: request.BusinessName},
				{InternalBusinessCode: "BUSINESS-2", BusinessName: request.BusinessName},
			}}, nil
		},
		detail: unexpectedDetail(t), searchGallery: unexpectedGallerySearch(t), galleryDetail: unexpectedGalleryDetail(t),
	}

	summary, err := newTestService(t, store, source).SyncJob(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Resolved != 0 || summary.Unresolved != 1 || len(store.saved) != 0 {
		t.Fatalf("summary=%+v saved=%+v", summary, store.saved)
	}
}

func TestService_SyncJob_공식화면오류를성공으로숨기지않는다(t *testing.T) {
	want := errors.New("official page unavailable")
	store := &fakeStore{groups: []PendingGroup{{BusinessName: "오류 상호", ProcessDate: time.Now(), RCNOs: []string{"RCNO-E"}}}}
	source := fakeSource{
		search: func(mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) { return mfdscompany.SearchPage{}, want },
		detail: unexpectedDetail(t), searchGallery: unexpectedGallerySearch(t), galleryDetail: unexpectedGalleryDetail(t),
	}

	_, err := newTestService(t, store, source).SyncJob(context.Background(), 10)
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}

func sourceMetadata(name string) mfdscompany.SourceMetadata {
	return mfdscompany.SourceMetadata{
		RequestURL:  "https://impfood.mfds.go.kr/" + name,
		RetrievedAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		BodySHA256:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
}

func unexpectedDetail(t *testing.T) func(string) (mfdscompany.BusinessDetail, error) {
	return func(string) (mfdscompany.BusinessDetail, error) {
		t.Fatal("unexpected detail call")
		return mfdscompany.BusinessDetail{}, nil
	}
}

func unexpectedGallerySearch(t *testing.T) func(mfdscompany.GallerySearchRequest) (mfdscompany.GalleryPage, error) {
	return func(mfdscompany.GallerySearchRequest) (mfdscompany.GalleryPage, error) {
		t.Fatal("unexpected gallery search call")
		return mfdscompany.GalleryPage{}, nil
	}
}

func unexpectedGalleryDetail(t *testing.T) func(string) (mfdscompany.GalleryDetail, error) {
	return func(string) (mfdscompany.GalleryDetail, error) {
		t.Fatal("unexpected gallery detail call")
		return mfdscompany.GalleryDetail{}, nil
	}
}
