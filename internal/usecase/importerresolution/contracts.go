package importerresolution

import (
	"context"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
)

type PendingGroup struct {
	BusinessName string
	ProcessDate  time.Time
	RCNOs        []string
}

type Resolution struct {
	RCNO          string
	BusinessName  string
	Importer      mfdscompany.BusinessDetail
	ListSource    mfdscompany.SourceMetadata
	GallerySource *mfdscompany.SourceMetadata
}

type Summary struct {
	Groups     int
	RCNOs      int
	Resolved   int
	Unresolved int
}

type Store interface {
	ListPendingImporterGroups(context.Context, uint64) ([]PendingGroup, error)
	SaveImporterResolutions(context.Context, []Resolution) error
}

type Source interface {
	Search(context.Context, mfdscompany.SearchRequest) (mfdscompany.SearchPage, error)
	Detail(context.Context, string) (mfdscompany.BusinessDetail, error)
	SearchGallery(context.Context, mfdscompany.GallerySearchRequest) (mfdscompany.GalleryPage, error)
	GalleryDetail(context.Context, string) (mfdscompany.GalleryDetail, error)
}

type Options struct {
	PageSize int
	Delay    time.Duration
	Industry string
	State    string
}
