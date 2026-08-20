package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/importerresolution"
)

func (s *Store) ListPendingImporterGroups(ctx context.Context, jobID uint64) ([]importerresolution.PendingGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT TRIM(item.importer_name), item.processed_date, item.rcno
		FROM mfds_items AS item
		LEFT JOIN mfds_importer_rcno_links AS link
		  ON link.rcno = item.rcno
		 AND CAST(link.source_importer_name AS BINARY) = CAST(TRIM(item.importer_name) AS BINARY)
		WHERE item.job_id = ?
		  AND NULLIF(TRIM(item.importer_name), '') IS NOT NULL
		  AND link.rcno IS NULL
		GROUP BY CAST(TRIM(item.importer_name) AS BINARY), item.processed_date, item.rcno
		ORDER BY item.processed_date, CAST(TRIM(item.importer_name) AS BINARY), item.rcno
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("수입사 해소 대상 조회 실패: %w", err)
	}
	defer rows.Close()
	groups := make([]importerresolution.PendingGroup, 0)
	for rows.Next() {
		var name, rcno string
		var processedDate sql.NullTime
		if err := rows.Scan(&name, &processedDate, &rcno); err != nil {
			return nil, fmt.Errorf("수입사 해소 대상 scan 실패: %w", err)
		}
		date := time.Time{}
		if processedDate.Valid {
			date = processedDate.Time
		}
		last := len(groups) - 1
		if last < 0 || groups[last].BusinessName != name || !groups[last].ProcessDate.Equal(date) {
			groups = append(groups, importerresolution.PendingGroup{BusinessName: name, ProcessDate: date})
			last++
		}
		groups[last].RCNOs = append(groups[last].RCNOs, rcno)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("수입사 해소 대상 순회 실패: %w", err)
	}
	return groups, nil
}

func (s *Store) SaveImporterResolutions(ctx context.Context, resolutions []importerresolution.Resolution) error {
	if len(resolutions) == 0 {
		return nil
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, resolution := range resolutions {
			if strings.TrimSpace(resolution.RCNO) == "" || strings.TrimSpace(resolution.BusinessName) == "" ||
				strings.TrimSpace(resolution.Importer.InternalBusinessCode) == "" ||
				strings.TrimSpace(resolution.Importer.BusinessName) == "" || resolution.ListSource.RetrievedAt.IsZero() {
				return fmt.Errorf("확정 수입사 연결 필드가 유효하지 않습니다")
			}
			importerID, err := upsertResolvedImporter(ctx, tx, resolution)
			if err != nil {
				return err
			}
			var galleryURL any
			var galleryHash any
			var galleryObservedAt any = resolution.ListSource.RetrievedAt
			linkSource := "PAGE_NAME"
			if resolution.GallerySource != nil {
				linkSource = "PAGE_RCNO"
				galleryURL = nullableString(resolution.GallerySource.RequestURL)
				galleryHash, err = decodeSourceHash(resolution.GallerySource.BodySHA256)
				if err != nil {
					return err
				}
				galleryObservedAt = resolution.GallerySource.RetrievedAt
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO mfds_importer_rcno_links (
					rcno, importer_id, source_importer_name, link_source,
					source_gallery_url, source_gallery_sha256, source_observed_at
				) VALUES (?, ?, ?, ?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE
					importer_id = VALUES(importer_id),
					source_importer_name = VALUES(source_importer_name),
					link_source = VALUES(link_source),
					source_gallery_url = VALUES(source_gallery_url),
					source_gallery_sha256 = VALUES(source_gallery_sha256),
					source_observed_at = VALUES(source_observed_at)
			`, resolution.RCNO, importerID, resolution.BusinessName, linkSource,
				galleryURL, galleryHash, galleryObservedAt)
			if err != nil {
				return fmt.Errorf("RCNO 수입사 증빙 저장 실패: %w", err)
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE mfds_declarations
				SET importer_id = ?, importer_link_source = ?, importer_linked_at = NOW(6)
				WHERE rcno = ?
				  AND NOT (importer_id <=> ? AND importer_link_source <=> ?)
			`, importerID, linkSource, resolution.RCNO, importerID, linkSource)
			if err != nil {
				return fmt.Errorf("정제 원장 수입사 연결 갱신 실패: %w", err)
			}
		}
		return nil
	})
}

func upsertResolvedImporter(ctx context.Context, tx *sql.Tx, resolution importerresolution.Resolution) (uint64, error) {
	detail := resolution.Importer
	name := strings.TrimSpace(detail.BusinessName)
	nameKey := sha256.Sum256([]byte(name))
	listHash, err := decodeSourceHash(resolution.ListSource.BodySHA256)
	if err != nil {
		return 0, err
	}
	var detailHash any
	var detailURL any
	if !detail.Source.RetrievedAt.IsZero() {
		detailHash, err = decodeSourceHash(detail.Source.BodySHA256)
		if err != nil {
			return 0, err
		}
		detailURL = nullableString(detail.Source.RequestURL)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO mfds_importers (
			official_business_code, license_no, business_name, business_name_key_sha256,
			representative_name, permit_date, institution_name, primary_address,
			telephone_no, industry_name, operating_status, source_list_url,
			source_detail_url, source_list_sha256, source_detail_sha256, source_observed_at
		) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			id = LAST_INSERT_ID(id), license_no = VALUES(license_no), business_name = VALUES(business_name),
			business_name_key_sha256 = VALUES(business_name_key_sha256),
			representative_name = VALUES(representative_name), permit_date = VALUES(permit_date),
			institution_name = VALUES(institution_name), primary_address = VALUES(primary_address),
			telephone_no = VALUES(telephone_no), industry_name = VALUES(industry_name),
			operating_status = VALUES(operating_status), source_list_url = VALUES(source_list_url),
			source_detail_url = VALUES(source_detail_url), source_list_sha256 = VALUES(source_list_sha256),
			source_detail_sha256 = VALUES(source_detail_sha256), source_observed_at = VALUES(source_observed_at)
	`, detail.InternalBusinessCode, detail.LicenseNumber, name, nameKey[:],
		nullString(detail.Representative), detail.PermitDate, nullString(detail.Institution),
		nullString(detail.Address), nullString(detail.Phone), nullString(detail.Industry),
		fallback(detail.OperatingStatus, "UNKNOWN"), resolution.ListSource.RequestURL,
		detailURL, listHash, detailHash, resolution.ListSource.RetrievedAt)
	if err != nil {
		return 0, fmt.Errorf("확정 수입사 저장 실패: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("확정 수입사 ID 확인 실패: id=%d: %w", id, err)
	}
	return uint64(id), nil
}

func decodeSourceHash(value string) ([]byte, error) {
	digest, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil || len(digest) != sha256.Size {
		return nil, fmt.Errorf("공식 페이지 SHA-256이 유효하지 않습니다")
	}
	return digest, nil
}
