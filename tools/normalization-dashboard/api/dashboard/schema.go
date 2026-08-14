package dashboard

import (
	"context"
	"fmt"
)

const requiredSchemaSQL = `SELECT
	(SELECT COUNT(*) FROM information_schema.tables
	 WHERE table_schema = DATABASE()
	   AND table_name IN (
		'mfds_importers', 'mfds_missing_importers'
	)),
	(SELECT COUNT(*) FROM information_schema.columns
	 WHERE table_schema = DATABASE() AND table_name = 'mfds_declarations'
	   AND column_name IN ('importer_id', 'importer_link_source')),
	(SELECT COUNT(*) FROM information_schema.columns
	 WHERE table_schema = DATABASE() AND table_name = 'mfds_declaration_details'
	   AND column_name IN ('importer_id', 'importer_link_source'))`

// ValidateRequiredSchema fails before serving requests when Flyway V12 is incomplete.
func ValidateRequiredSchema(ctx context.Context, queryer Queryer) error {
	rows, err := queryer.QueryContext(ctx, requiredSchemaSQL)
	if err != nil {
		return fmt.Errorf("dashboard schema check failed: %w", err)
	}
	var importerObjects, declarationColumns, detailViewColumns int
	if err := scanSingle(rows, &importerObjects, &declarationColumns, &detailViewColumns); err != nil {
		return fmt.Errorf("dashboard schema check failed: %w", err)
	}
	if importerObjects != 2 || declarationColumns != 2 || detailViewColumns != 2 {
		return fmt.Errorf("importer dashboard schema is required: importer_tables=%d declaration_importer_columns=%d detail_view_importer_columns=%d", importerObjects, declarationColumns, detailViewColumns)
	}
	return nil
}
