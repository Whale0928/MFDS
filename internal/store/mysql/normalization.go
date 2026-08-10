package mysql

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bottle-note/mfds-crawler/git.secrets/project/mfds/sqlcgen"
	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

var errNormalizationLeaseLost = errors.New("normalization claim lease lost")

// SyncDeclarations refreshes each RCNO's logical source reference without
// modifying immutable ledger rows.
func (s *Store) SyncDeclarations(ctx context.Context) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO declarations (rcno, source_item_id)
			SELECT latest.rcno, latest.id
			FROM (
				SELECT id, rcno,
				       ROW_NUMBER() OVER (
				           PARTITION BY rcno ORDER BY observed_at DESC, id DESC
				       ) AS rn
				FROM items
			) AS latest
			WHERE latest.rn = 1
			ON DUPLICATE KEY UPDATE rcno = declarations.rcno
		`)
		if err != nil {
			return fmt.Errorf("신규 declaration 동기화 실패: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE declarations AS d
			JOIN items AS previous ON previous.id = d.source_item_id
			JOIN (
				SELECT id, rcno
				FROM (
					SELECT id, rcno,
					       ROW_NUMBER() OVER (
					           PARTITION BY rcno ORDER BY observed_at DESC, id DESC
					       ) AS rn
					FROM items
				) AS ranked
				WHERE ranked.rn = 1
			) AS latest ON latest.rcno = d.rcno
			JOIN items AS current ON current.id = latest.id
			SET d.normalization_status = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256
			        THEN d.normalization_status
			        ELSE 'STALE'
			    END,
			    d.claim_owner = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.claim_owner ELSE NULL
			    END,
			    d.claim_lease_until = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.claim_lease_until ELSE NULL
			    END,
			    d.claim_attempts = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.claim_attempts ELSE 0
			    END,
			    d.claim_next_attempt_at = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.claim_next_attempt_at ELSE NULL
			    END,
			    d.claim_last_error = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.claim_last_error ELSE NULL
			    END,
			    d.review_status = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.review_status ELSE 'NOT_REQUIRED'
			    END,
			    d.reviewed_by = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.reviewed_by ELSE NULL
			    END,
			    d.reviewed_at = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.reviewed_at ELSE NULL
			    END,
			    d.review_note = CASE
			        WHEN previous.semantic_sha256 <=> current.semantic_sha256 THEN d.review_note ELSE NULL
			    END,
			    d.source_item_id = latest.id
			WHERE d.source_item_id <> latest.id
		`)
		if err != nil {
			return fmt.Errorf("declaration 최신 원본 갱신 실패: %w", err)
		}
		return nil
	})
}

// Claim atomically leases PENDING/STALE declarations. An RCNO request ignores
// normalization status but never steals an unexpired claim.
func (s *Store) Claim(
	ctx context.Context,
	request normalization.ClaimRequest,
) ([]normalization.Source, error) {
	if err := validateNormalizationClaimRequest(request); err != nil {
		return nil, err
	}
	// lease 만료는 DB 시계(NOW(6), Asia/Seoul)로만 계산한다. 앱 호스트 시계로 계산하면
	// 비교 대상인 NOW(6)와 출처가 달라져 시계 차이만큼 lease가 일찍 끊기거나 늦게 풀린다.
	leaseMicroseconds := request.LeaseDuration.Microseconds()
	var sources []normalization.Source
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT d.id, d.rcno, d.source_item_id, HEX(i.semantic_sha256),
			       COALESCE(i.product_name_ko, ''), COALESCE(i.product_name_en, ''),
			       COALESCE(i.item_name, ''), COALESCE(i.importer_name, ''),
			       COALESCE(i.overseas_establishment_name, ''),
			       COALESCE(i.manufacture_country_name, ''), COALESCE(i.export_country_name, ''),
			       COALESCE(i.expiry_text, ''),
			       i.processed_date, d.claim_attempts
			FROM declarations AS d
			JOIN items AS i ON i.id = d.source_item_id
			WHERE (d.claim_owner IS NULL OR d.claim_lease_until < NOW(6))
			  AND (
			       (? <> '' AND d.rcno = ?)
			       OR (
			           ? = ''
			           AND d.normalization_status IN ('PENDING', 'STALE')
			           AND (d.claim_next_attempt_at IS NULL OR d.claim_next_attempt_at <= NOW(6))
			           AND d.claim_attempts < ?
			       )
			  )
			ORDER BY i.processed_date DESC, d.source_item_id DESC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		`, request.RCNO, request.RCNO, request.RCNO, request.MaxAttempts, request.Limit)
		if err != nil {
			return fmt.Errorf("normalization claim 후보 조회 실패: %w", err)
		}
		type candidate struct {
			source   normalization.Source
			attempts int
		}
		candidates := make([]candidate, 0, request.Limit)
		for rows.Next() {
			source, attempts, scanErr := scanNormalizationSource(rows)
			if scanErr != nil {
				_ = rows.Close()
				return fmt.Errorf("normalization claim 후보 scan 실패: %w", scanErr)
			}
			candidates = append(candidates, candidate{source: source, attempts: attempts})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("normalization claim 후보 순회 실패: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("normalization claim 후보 close 실패: %w", err)
		}
		for _, candidate := range candidates {
			source := candidate.source
			result, updateErr := tx.ExecContext(ctx, `
				UPDATE declarations
				SET claim_owner = ?, claim_lease_until = NOW(6) + INTERVAL ? MICROSECOND,
				    claim_attempts = claim_attempts + 1,
				    claim_next_attempt_at = NULL, claim_last_error = NULL
				WHERE id = ? AND claim_attempts = ?
			`, request.Owner, leaseMicroseconds, source.DeclarationID, candidate.attempts)
			if updateErr != nil {
				return fmt.Errorf("normalization claim lease 설정 실패: %w", updateErr)
			}
			if err := requireOne(result, "normalization claim lease 설정"); err != nil {
				return err
			}
			source.ClaimOwner = request.Owner
			source.ClaimAttempt = candidate.attempts + 1
			sources = append(sources, source)
		}
		return nil
	})
	return sources, err
}

// Preview returns the same ordering as Claim without synchronizing, leasing,
// changing attempts, or otherwise writing to the database.
func (s *Store) Preview(
	ctx context.Context,
	request normalization.ClaimRequest,
) ([]normalization.Source, error) {
	if err := validateNormalizationClaimRequest(request); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH latest_items AS (
			SELECT id, rcno, semantic_sha256, product_name_ko, product_name_en,
			       item_name, importer_name, overseas_establishment_name,
			       manufacture_country_name, export_country_name, expiry_text,
			       processed_date,
			       ROW_NUMBER() OVER (
			           PARTITION BY rcno ORDER BY observed_at DESC, id DESC
			       ) AS rn
			FROM items
		)
		SELECT COALESCE(d.id, 0), latest.rcno, latest.id, HEX(latest.semantic_sha256),
		       COALESCE(latest.product_name_ko, ''), COALESCE(latest.product_name_en, ''),
		       COALESCE(latest.item_name, ''), COALESCE(latest.importer_name, ''),
		       COALESCE(latest.overseas_establishment_name, ''),
		       COALESCE(latest.manufacture_country_name, ''), COALESCE(latest.export_country_name, ''),
		       COALESCE(latest.expiry_text, ''),
		       latest.processed_date, COALESCE(d.claim_attempts, 0)
		FROM latest_items AS latest
		LEFT JOIN declarations AS d ON d.rcno = latest.rcno
		WHERE latest.rn = 1
		  AND (d.claim_owner IS NULL OR d.claim_lease_until < NOW(6))
		  AND (
		      (? <> '' AND latest.rcno = ?)
		      OR (
		          ? = ''
		          AND (
		              d.id IS NULL
		              OR (
		                  d.normalization_status IN ('PENDING', 'STALE')
		                  AND (d.claim_next_attempt_at IS NULL OR d.claim_next_attempt_at <= NOW(6))
		                  AND d.claim_attempts < ?
		              )
		          )
		      )
		  )
		ORDER BY latest.processed_date DESC, latest.id DESC
		LIMIT ?
	`, request.RCNO, request.RCNO, request.RCNO, request.MaxAttempts, request.Limit)
	if err != nil {
		return nil, fmt.Errorf("normalization preview 후보 조회 실패: %w", err)
	}
	defer rows.Close()

	sources := make([]normalization.Source, 0)
	for rows.Next() {
		source, _, scanErr := scanNormalizationSource(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("normalization preview 후보 scan 실패: %w", scanErr)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("normalization preview 후보 순회 실패: %w", err)
	}
	return sources, nil
}

// Complete persists only derived values after validating the claim fence.
func (s *Store) Complete(ctx context.Context, completion normalization.Completion) error {
	if !completion.Result.Status.IsTerminalDataStatus() {
		return fmt.Errorf("terminal normalization status가 아닙니다: %s", completion.Result.Status)
	}
	key, err := nullableSHA256(completion.Result.Fields.SKUCandidateKeySHA256)
	if err != nil {
		return err
	}
	reasons, err := normalizationJSON(completion.Result.Reasons)
	if err != nil {
		return err
	}
	fragments, err := normalizationJSON(completion.Result.UnparsedFragments)
	if err != nil {
		return err
	}
	fields := completion.Result.Fields
	status := string(completion.Result.Status)

	assign := newColumnAssignments(60)
	assign.set("base_product_name_ko", nullableString(fields.BaseProductNameKO))
	assign.set("base_product_name_en", nullableString(fields.BaseProductNameEN))
	assign.set("sku_display_name_ko", nullableString(fields.SKUDisplayNameKO))
	assign.set("sku_display_name_en", nullableString(fields.SKUDisplayNameEN))
	assign.set("name_search_key_ko", nullableString(fields.NameSearchKeyKO))
	assign.set("name_search_key_en", nullableString(fields.NameSearchKeyEN))
	assign.set("sku_candidate_key_sha256", key)

	assign.set("volume_raw", nullableString(fields.VolumeRaw))
	assign.set("volume_ml", nullableInt(fields.VolumeML))
	assign.set("unit_volume_ml", nullableInt(fields.UnitVolumeML))
	assign.set("package_count", nullableInt(fields.PackageCount))
	assign.set("abv_raw", nullableString(fields.ABVRaw))
	assign.set("abv_percent", nullableFloat(fields.ABVPercent))
	assign.set("ingredient_percent_raw", nullableString(fields.IngredientPercentRaw))
	assign.set("ingredient_percent", nullableFloat(fields.IngredientPercent))
	assign.set("proof_raw", nullableString(fields.ProofRaw))
	assign.set("proof_value", nullableFloat(fields.ProofValue))
	assign.set("strength_type", nullableString(fields.StrengthType))
	assign.set("age_raw", nullableString(fields.AgeRaw))
	assign.set("age_years", nullableInt(fields.AgeYears))
	assign.set("vintage_raw", nullableString(fields.VintageRaw))
	assign.set("vintage_year", nullableInt(fields.VintageYear))
	assign.set("version_marker", nullableString(fields.VersionMarker))
	assign.set("edition_name", nullableString(fields.EditionName))
	assign.set("variant_marker_raw", nullableString(fields.VariantMarkerRaw))
	assign.set("variant_marker_type", nullableString(fields.VariantMarkerType))
	assign.set("variant_marker_value", nullableString(fields.VariantMarkerValue))
	assign.set("material_code", nullableString(fields.MaterialCode))
	assign.set("cask_number", nullableString(fields.CaskNumber))
	assign.set("batch_number", nullableString(fields.BatchNumber))
	assign.set("lot_number", nullableString(fields.LotNumber))
	assign.set("manufacture_number", nullableString(fields.ManufactureNumber))

	assign.set("expiry_raw", nullableString(fields.ExpiryRaw))
	assign.set("expiry_start", nullableTime(fields.ExpiryStart))
	assign.set("expiry_end", nullableTime(fields.ExpiryEnd))

	assign.set("importer_base_name", nullableString(fields.ImporterBaseName))
	assign.set("importer_search_key", nullableString(fields.ImporterSearchKey))
	assign.set("legal_entity_type", nullableString(fields.LegalEntityType))
	assign.set("manufacturer_name", nullableString(fields.ManufacturerName))
	assign.set("overseas_establishment_search_key", nullableString(fields.OverseasEstablishmentSearchKey))

	assign.set("alcohol_name_ko", nullableString(fields.AlcoholNameKO))
	assign.set("alcohol_name_en", nullableString(fields.AlcoholNameEN))
	assign.set("alcohol_category_ko", nullableString(fields.AlcoholCategoryKO))
	assign.set("alcohol_category_en", nullableString(fields.AlcoholCategoryEN))
	assign.set("alcohol_region_ko", nullableString(fields.AlcoholRegionKO))
	assign.set("alcohol_region_en", nullableString(fields.AlcoholRegionEN))
	assign.set("alcohol_abv", nullableString(fields.AlcoholABV))
	assign.set("cask_candidate", nullableString(fields.CaskCandidate))
	assign.set("distillery_name_ko_candidate", nullableString(fields.DistilleryNameKOCandidate))
	assign.set("distillery_name_en_candidate", nullableString(fields.DistilleryNameENCandidate))

	assign.set("manufacture_country_name_ko", nullableString(fields.ManufactureCountryNameKO))
	assign.set("manufacture_country_name_en", nullableString(fields.ManufactureCountryNameEN))
	assign.set("manufacture_country_alpha2", nullableString(fields.ManufactureCountryAlpha2))
	assign.set("manufacture_country_alpha3", nullableString(fields.ManufactureCountryAlpha3))
	assign.set("export_country_name_ko", nullableString(fields.ExportCountryNameKO))
	assign.set("export_country_name_en", nullableString(fields.ExportCountryNameEN))
	assign.set("export_country_alpha2", nullableString(fields.ExportCountryAlpha2))
	assign.set("export_country_alpha3", nullableString(fields.ExportCountryAlpha3))

	assign.set("normalization_status", status)
	assign.set("normalization_version", nullableString(completion.NormalizationVersion))
	assign.set("normalization_reasons", reasons)
	assign.set("unparsed_fragments_json", fragments)
	assign.set("normalized_at", completion.NormalizedAt)

	assign.setExpression("claim_owner = NULL")
	assign.setExpression("claim_lease_until = NULL")
	assign.setExpression("claim_next_attempt_at = NULL")
	assign.setExpression("claim_last_error = NULL")
	assign.setExpression(`review_status = CASE
		        WHEN ? = 'REVIEW_REQUIRED' THEN 'PENDING'
		        ELSE 'NOT_REQUIRED'
		    END`, status)

	result, err := s.db.ExecContext(ctx, `
		UPDATE declarations
		SET `+assign.clause()+`
		WHERE id = ? AND rcno = ? AND source_item_id = ?
		  AND claim_owner = ? AND claim_attempts = ? AND claim_lease_until >= NOW(6)
	`, assign.arguments(
		completion.Source.DeclarationID, completion.Source.RCNO, completion.Source.SourceItemID,
		completion.Source.ClaimOwner, completion.Source.ClaimAttempt,
	)...)
	if err != nil {
		return fmt.Errorf("normalization 결과 저장 실패: %w", err)
	}
	if err := requireNormalizationLease(result, "normalization 결과 저장"); err != nil {
		return err
	}
	return nil
}

// Fail releases a fenced claim for a later retry while keeping prior derived
// values intact.
func (s *Store) Fail(ctx context.Context, failure normalization.Failure) error {
	// 재시도 시각도 lease와 같은 DB 시계를 쓴다. 이 값은 Claim의 NOW(6) 비교 대상이다.
	result, err := s.db.ExecContext(ctx, `
		UPDATE declarations
		SET claim_owner = NULL, claim_lease_until = NULL,
		    claim_next_attempt_at = NOW(6) + INTERVAL ? MICROSECOND, claim_last_error = ?
		WHERE id = ? AND rcno = ? AND source_item_id = ?
		  AND claim_owner = ? AND claim_attempts = ? AND claim_lease_until >= NOW(6)
	`, failure.RetryDelay.Microseconds(), truncateNormalizationError(failure.LastError),
		failure.Source.DeclarationID, failure.Source.RCNO, failure.Source.SourceItemID,
		failure.Source.ClaimOwner, failure.Source.ClaimAttempt)
	if err != nil {
		return fmt.Errorf("normalization 실패 상태 저장 실패: %w", err)
	}
	return requireNormalizationLease(result, "normalization 실패 상태 저장")
}

// Remaining includes ledger RCNOs not yet materialized as declarations so a
// dry-run summary remains correct before the first synchronization.
func (s *Store) Remaining(ctx context.Context) (normalization.Remaining, error) {
	row, err := sqlcgen.New(s.db).GetNormalizationRemaining(ctx)
	if err != nil {
		return normalization.Remaining{}, fmt.Errorf("normalization 잔여 declaration 조회 실패: %w", err)
	}
	var missing int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT rcno FROM items GROUP BY rcno
		) AS ledger_rcno
		LEFT JOIN declarations AS d ON d.rcno = ledger_rcno.rcno
		WHERE d.id IS NULL
	`).Scan(&missing)
	if err != nil {
		return normalization.Remaining{}, fmt.Errorf("normalization 미동기화 RCNO 조회 실패: %w", err)
	}
	return normalization.Remaining{Pending: int(row.Pending) + missing, Stale: int(row.Stale)}, nil
}

func validateNormalizationClaimRequest(request normalization.ClaimRequest) error {
	if request.Limit <= 0 || request.MaxAttempts <= 0 || request.LeaseDuration <= 0 {
		return errors.New("normalization claim limit, lease, max attempts는 모두 0보다 커야 합니다")
	}
	if strings.TrimSpace(request.Owner) == "" {
		return errors.New("normalization claim owner가 필요합니다")
	}
	return nil
}

type normalizationSourceScanner interface {
	Scan(...any) error
}

func scanNormalizationSource(scanner normalizationSourceScanner) (normalization.Source, int, error) {
	var source normalization.Source
	var processedDate sql.NullTime
	var attempts int
	err := scanner.Scan(
		&source.DeclarationID, &source.RCNO, &source.SourceItemID, &source.SemanticHash,
		&source.ProductNameKO, &source.ProductNameEN, &source.ItemName, &source.ImporterName,
		&source.OverseasEstablishmentName, &source.ManufactureCountryName, &source.ExportCountryName,
		&source.ExpiryText, &processedDate, &attempts,
	)
	if err != nil {
		return normalization.Source{}, 0, err
	}
	if processedDate.Valid {
		source.ProcessedDate = processedDate.Time
	}
	return source, attempts, nil
}

func nullableSHA256(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("sku candidate SHA-256은 64자리 hex 문자열이어야 합니다")
	}
	return decoded, nil
}

func normalizationJSON(values []string) ([]byte, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("normalization JSON 인코딩 실패: %w", err)
	}
	return encoded, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func truncateNormalizationError(value string) string {
	const maximum = 4096
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func requireNormalizationLease(result sql.Result, operation string) error {
	if err := requireOne(result, operation); err != nil {
		return fmt.Errorf("%w: %v", errNormalizationLeaseLost, err)
	}
	return nil
}

var _ normalization.Store = (*Store)(nil)
