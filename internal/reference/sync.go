// Package reference mirrors BottleNote matching data into the local MFDS database.
package reference

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Result is the post-sync, source-to-target comparison. Hashes are deterministic row snapshots.
type Result struct {
	Regions, Distilleries, Alcohols TableResult
}

type TableResult struct {
	Count int
	Hash  string
}

type region struct {
	ID                     int64
	KorName, EngName       string
	Continent, Description sql.NullString
	CreateAt, LastModifyAt sql.NullTime
	CreateEmail, LastEmail sql.NullString
	CreateID, LastID       sql.NullInt64
	CreateType, LastType   sql.NullString
	ParentID               sql.NullInt64
	SortOrder              int64
	ImageURL               sql.NullString
}
type distillery struct {
	ID                     int64
	KorName, EngName       string
	ImageURL               sql.NullString
	CreateAt, LastModifyAt sql.NullTime
	CreateEmail, LastEmail sql.NullString
	Description            sql.NullString
	SortOrder              int64
	CreateID, LastID       sql.NullInt64
	CreateType, LastType   sql.NullString
}
type alcohol struct {
	ID                                            int64
	KorName, EngName                              string
	ABV                                           sql.NullString
	Type, KorCategory, EngCategory, CategoryGroup string
	RegionID, DistilleryID                        sql.NullInt64
	Age, Cask, ImageURL, Description, Volume      sql.NullString
	CreateAt, LastModifyAt, DeletedAt             sql.NullTime
	CreateEmail, LastEmail                        sql.NullString
	CreateID, LastID                              sql.NullInt64
	CreateType, LastType                          sql.NullString
}

// Sync reads all source rows in one consistent read, replaces the target mirror atomically, then compares hashes.
func Sync(ctx context.Context, source, target *sql.DB) (Result, error) {
	sourceTx, err := source.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return Result{}, fmt.Errorf("reference source consistent read 시작 실패: %w", err)
	}
	defer sourceTx.Rollback()
	regions, err := readRegions(ctx, sourceTx)
	if err != nil {
		return Result{}, err
	}
	distilleries, err := readDistilleries(ctx, sourceTx)
	if err != nil {
		return Result{}, err
	}
	alcohols, err := readAlcohols(ctx, sourceTx)
	if err != nil {
		return Result{}, err
	}
	if err := sourceTx.Commit(); err != nil {
		return Result{}, fmt.Errorf("reference source consistent read 완료 실패: %w", err)
	}

	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("reference target transaction 시작 실패: %w", err)
	}
	if err := replaceTarget(ctx, tx, regions, distilleries, alcohols); err != nil {
		_ = tx.Rollback()
		return Result{}, err
	}
	sourceResult := resultFor(regions, distilleries, alcohols)
	verifiedRegions, err := readRegions(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return Result{}, fmt.Errorf("reference target regions 검증 실패: %w", err)
	}
	verifiedDistilleries, err := readDistilleries(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return Result{}, fmt.Errorf("reference target distilleries 검증 실패: %w", err)
	}
	verifiedAlcohols, err := readAlcohols(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return Result{}, fmt.Errorf("reference target alcohols 검증 실패: %w", err)
	}
	verified := resultFor(verifiedRegions, verifiedDistilleries, verifiedAlcohols)
	if verified != sourceResult {
		_ = tx.Rollback()
		return Result{}, fmt.Errorf("reference target 검증 불일치: source=%+v target=%+v", sourceResult, verified)
	}
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("reference target transaction 완료 실패: %w", err)
	}
	return verified, nil
}

func resultFor(regions []region, distilleries []distillery, alcohols []alcohol) Result {
	return Result{Regions: TableResult{len(regions), hashRows(regionLines(regions))}, Distilleries: TableResult{len(distilleries), hashRows(distilleryLines(distilleries))}, Alcohols: TableResult{len(alcohols), hashRows(alcoholLines(alcohols))}}
}

func readRegions(ctx context.Context, tx *sql.Tx) ([]region, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,kor_name,eng_name,continent,description,create_at,create_principal_email,last_modify_at,last_modify_principal_email,parent_id,sort_order,image_url,create_principal_id,create_principal_type,last_modify_principal_id,last_modify_principal_type FROM regions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("source regions 조회 실패: %w", err)
	}
	defer rows.Close()
	var values []region
	for rows.Next() {
		var v region
		if err := rows.Scan(&v.ID, &v.KorName, &v.EngName, &v.Continent, &v.Description, &v.CreateAt, &v.CreateEmail, &v.LastModifyAt, &v.LastEmail, &v.ParentID, &v.SortOrder, &v.ImageURL, &v.CreateID, &v.CreateType, &v.LastID, &v.LastType); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
func readDistilleries(ctx context.Context, tx *sql.Tx) ([]distillery, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,kor_name,eng_name,image_url,create_at,create_principal_email,last_modify_at,last_modify_principal_email,description,sort_order,create_principal_id,create_principal_type,last_modify_principal_id,last_modify_principal_type FROM distilleries ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("source distilleries 조회 실패: %w", err)
	}
	defer rows.Close()
	var values []distillery
	for rows.Next() {
		var v distillery
		if err := rows.Scan(&v.ID, &v.KorName, &v.EngName, &v.ImageURL, &v.CreateAt, &v.CreateEmail, &v.LastModifyAt, &v.LastEmail, &v.Description, &v.SortOrder, &v.CreateID, &v.CreateType, &v.LastID, &v.LastType); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
func readAlcohols(ctx context.Context, tx *sql.Tx) ([]alcohol, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,kor_name,eng_name,abv,type,kor_category,eng_category,category_group,region_id,distillery_id,age,cask,image_url,description,volume,create_at,create_principal_email,create_principal_id,create_principal_type,last_modify_at,last_modify_principal_email,last_modify_principal_id,last_modify_principal_type,deleted_at FROM alcohols ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("source alcohols 조회 실패: %w", err)
	}
	defer rows.Close()
	var values []alcohol
	for rows.Next() {
		var v alcohol
		if err := rows.Scan(&v.ID, &v.KorName, &v.EngName, &v.ABV, &v.Type, &v.KorCategory, &v.EngCategory, &v.CategoryGroup, &v.RegionID, &v.DistilleryID, &v.Age, &v.Cask, &v.ImageURL, &v.Description, &v.Volume, &v.CreateAt, &v.CreateEmail, &v.CreateID, &v.CreateType, &v.LastModifyAt, &v.LastEmail, &v.LastID, &v.LastType, &v.DeletedAt); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func replaceTarget(ctx context.Context, tx *sql.Tx, regions []region, distilleries []distillery, alcohols []alcohol) error {
	if _, err := tx.ExecContext(ctx, `SET SESSION sql_mode = CONCAT_WS(',', @@SESSION.sql_mode, 'NO_AUTO_VALUE_ON_ZERO')`); err != nil {
		return fmt.Errorf("target AUTO_INCREMENT 0 보존 설정 실패: %w", err)
	}
	for _, table := range []string{"alcohols", "distilleries", "regions"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("target %s 비우기 실패: %w", table, err)
		}
	}
	for _, v := range regions {
		_, err := tx.ExecContext(ctx, `INSERT INTO regions (id,kor_name,eng_name,continent,description,create_at,create_principal_email,last_modify_at,last_modify_principal_email,parent_id,sort_order,image_url,create_principal_id,create_principal_type,last_modify_principal_id,last_modify_principal_type) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.KorName, v.EngName, v.Continent, v.Description, v.CreateAt, v.CreateEmail, v.LastModifyAt, v.LastEmail, v.ParentID, v.SortOrder, v.ImageURL, v.CreateID, v.CreateType, v.LastID, v.LastType)
		if err != nil {
			return fmt.Errorf("target region %d 저장 실패: %w", v.ID, err)
		}
	}
	for _, v := range distilleries {
		_, err := tx.ExecContext(ctx, `INSERT INTO distilleries (id,kor_name,eng_name,image_url,create_at,create_principal_email,last_modify_at,last_modify_principal_email,description,sort_order,create_principal_id,create_principal_type,last_modify_principal_id,last_modify_principal_type) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.KorName, v.EngName, v.ImageURL, v.CreateAt, v.CreateEmail, v.LastModifyAt, v.LastEmail, v.Description, v.SortOrder, v.CreateID, v.CreateType, v.LastID, v.LastType)
		if err != nil {
			return fmt.Errorf("target distillery %d 저장 실패: %w", v.ID, err)
		}
	}
	for _, v := range alcohols {
		_, err := tx.ExecContext(ctx, `INSERT INTO alcohols (id,kor_name,eng_name,abv,type,kor_category,eng_category,category_group,region_id,distillery_id,age,cask,image_url,description,volume,create_at,create_principal_email,create_principal_id,create_principal_type,last_modify_at,last_modify_principal_email,last_modify_principal_id,last_modify_principal_type,deleted_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.KorName, v.EngName, v.ABV, v.Type, v.KorCategory, v.EngCategory, v.CategoryGroup, v.RegionID, v.DistilleryID, v.Age, v.Cask, v.ImageURL, v.Description, v.Volume, v.CreateAt, v.CreateEmail, v.CreateID, v.CreateType, v.LastModifyAt, v.LastEmail, v.LastID, v.LastType, v.DeletedAt)
		if err != nil {
			return fmt.Errorf("target alcohol %d 저장 실패: %w", v.ID, err)
		}
	}
	return nil
}

func hashRows(lines []string) string {
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
func regionLines(values []region) []string {
	lines := make([]string, len(values))
	for i, v := range values {
		lines[i] = canonical(
			integer(v.ID), text(v.KorName), text(v.EngName), nullText(v.Continent), nullText(v.Description),
			nullTimestamp(v.CreateAt), nullText(v.CreateEmail), nullTimestamp(v.LastModifyAt), nullText(v.LastEmail),
			nullInt(v.ParentID), integer(v.SortOrder), nullText(v.ImageURL), nullInt(v.CreateID), nullText(v.CreateType), nullInt(v.LastID), nullText(v.LastType),
		)
	}
	return lines
}
func distilleryLines(values []distillery) []string {
	lines := make([]string, len(values))
	for i, v := range values {
		lines[i] = canonical(
			integer(v.ID), text(v.KorName), text(v.EngName), nullText(v.ImageURL), nullTimestamp(v.CreateAt), nullText(v.CreateEmail),
			nullTimestamp(v.LastModifyAt), nullText(v.LastEmail), nullText(v.Description), integer(v.SortOrder), nullInt(v.CreateID), nullText(v.CreateType), nullInt(v.LastID), nullText(v.LastType),
		)
	}
	return lines
}
func alcoholLines(values []alcohol) []string {
	lines := make([]string, len(values))
	for i, v := range values {
		lines[i] = canonical(
			integer(v.ID), text(v.KorName), text(v.EngName), nullText(v.ABV), text(v.Type), text(v.KorCategory), text(v.EngCategory), text(v.CategoryGroup),
			nullInt(v.RegionID), nullInt(v.DistilleryID), nullText(v.Age), nullText(v.Cask), nullText(v.ImageURL), nullText(v.Description), nullText(v.Volume),
			nullTimestamp(v.CreateAt), nullText(v.CreateEmail), nullInt(v.CreateID), nullText(v.CreateType), nullTimestamp(v.LastModifyAt), nullText(v.LastEmail), nullInt(v.LastID), nullText(v.LastType), nullTimestamp(v.DeletedAt),
		)
	}
	return lines
}
func canonical(values ...string) string { return strings.Join(values, "\x1f") }
func integer(value int64) string        { return fmt.Sprintf("i:%d", value) }
func text(value string) string          { return fmt.Sprintf("s:%q", value) }
func nullText(value sql.NullString) string {
	if !value.Valid {
		return "s:null"
	}
	return text(value.String)
}
func nullInt(value sql.NullInt64) string {
	if !value.Valid {
		return "i:null"
	}
	return integer(value.Int64)
}
func nullTimestamp(value sql.NullTime) string {
	if !value.Valid {
		return "t:null"
	}
	return "t:" + value.Time.UTC().Format(time.RFC3339Nano)
}
