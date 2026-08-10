package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/config"
	parser "github.com/bottle-note/mfds-crawler/internal/normalization"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	usecase "github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

func runNormalization(
	ctx context.Context,
	cfg config.Config,
	command usecase.Command,
) (summary usecase.Summary, runErr error) {
	store, err := storemysql.Open(cfg.Database)
	if err != nil {
		return usecase.Summary{SystemFailures: 1}, err
	}
	defer func() {
		runErr = errors.Join(runErr, store.Close())
	}()

	service, err := usecase.NewService(store, parserAdapter{}, usecase.Options{
		RunLimit:             cfg.Normalization.RunLimit,
		LeaseDuration:        cfg.Normalization.LeaseDuration,
		MaxAttempts:          cfg.Normalization.MaxAttempts,
		RetryDelay:           cfg.Normalization.RetryDelay,
		NormalizationVersion: parser.Version,
		Now:                  time.Now,
		Owner:                normalizationOwner(),
	})
	if err != nil {
		return usecase.Summary{SystemFailures: 1}, err
	}
	return service.Execute(ctx, command)
}

type parserAdapter struct{}

func (parserAdapter) Normalize(source usecase.Source) (usecase.Result, error) {
	result := parser.Normalize(parser.Input{
		ProductNameKO:             source.ProductNameKO,
		ProductNameEN:             source.ProductNameEN,
		ItemName:                  source.ItemName,
		ImporterName:              source.ImporterName,
		OverseasEstablishmentName: source.OverseasEstablishmentName,
		ManufactureCountryName:    source.ManufactureCountryName,
		ExportCountryName:         source.ExportCountryName,
		ExpiryText:                source.ExpiryText,
	})
	reasons := make([]string, len(result.Reasons))
	for index, reason := range result.Reasons {
		reasons[index] = string(reason)
	}
	return usecase.Result{
		Status: usecase.Status(result.Status),
		Fields: usecase.Fields{
			BaseProductNameKO:              result.BaseProductNameKO,
			BaseProductNameEN:              result.BaseProductNameEN,
			SKUDisplayNameKO:               result.SKUDisplayNameKO,
			SKUDisplayNameEN:               result.SKUDisplayNameEN,
			NameSearchKeyKO:                result.NameSearchKeyKO,
			NameSearchKeyEN:                result.NameSearchKeyEN,
			SKUCandidateKeySHA256:          result.SKUCandidateKeySHA256,
			VolumeRaw:                      result.VolumeRaw,
			VolumeML:                       result.VolumeML,
			UnitVolumeML:                   result.UnitVolumeML,
			PackageCount:                   result.PackageCount,
			ABVRaw:                         result.ABVRaw,
			ABVPercent:                     result.ABVPercent,
			IngredientPercentRaw:           result.IngredientPercentRaw,
			IngredientPercent:              result.IngredientPercent,
			ProofRaw:                       result.ProofRaw,
			ProofValue:                     result.ProofValue,
			StrengthType:                   result.StrengthType,
			AgeRaw:                         result.AgeRaw,
			AgeYears:                       result.AgeYears,
			VintageRaw:                     result.VintageRaw,
			VintageYear:                    result.VintageYear,
			VersionMarker:                  result.VersionMarker,
			EditionName:                    result.EditionName,
			VariantMarkerRaw:               result.VariantMarkerRaw,
			VariantMarkerType:              result.VariantMarkerType,
			VariantMarkerValue:             result.VariantMarkerValue,
			MaterialCode:                   result.MaterialCode,
			CaskNumber:                     result.CaskNumber,
			BatchNumber:                    result.BatchNumber,
			LotNumber:                      result.LotNumber,
			ManufactureNumber:              result.ManufactureNumber,
			ExpiryRaw:                      result.ExpiryRaw,
			ExpiryStart:                    result.ExpiryStart,
			ExpiryEnd:                      result.ExpiryEnd,
			ImporterBaseName:               result.ImporterBaseName,
			ImporterSearchKey:              result.ImporterSearchKey,
			LegalEntityType:                result.LegalEntityType,
			ManufacturerName:               result.ManufacturerName,
			OverseasEstablishmentSearchKey: result.OverseasEstablishmentSearchKey,
			AlcoholNameKO:                  result.AlcoholNameKO,
			AlcoholNameEN:                  result.AlcoholNameEN,
			AlcoholCategoryKO:              result.AlcoholCategoryKO,
			AlcoholCategoryEN:              result.AlcoholCategoryEN,
			AlcoholRegionKO:                result.AlcoholRegionKO,
			AlcoholRegionEN:                result.AlcoholRegionEN,
			AlcoholABV:                     result.AlcoholABV,
			CaskCandidate:                  result.CaskCandidate,
			DistilleryNameKOCandidate:      result.DistilleryNameKOCandidate,
			DistilleryNameENCandidate:      result.DistilleryNameENCandidate,
			ManufactureCountryNameKO:       result.ManufactureCountry.NameKO,
			ManufactureCountryNameEN:       result.ManufactureCountry.NameEN,
			ManufactureCountryAlpha2:       result.ManufactureCountry.Alpha2,
			ManufactureCountryAlpha3:       result.ManufactureCountry.Alpha3,
			ExportCountryNameKO:            result.ExportCountry.NameKO,
			ExportCountryNameEN:            result.ExportCountry.NameEN,
			ExportCountryAlpha2:            result.ExportCountry.Alpha2,
			ExportCountryAlpha3:            result.ExportCountry.Alpha3,
		},
		Reasons:           reasons,
		UnparsedFragments: result.UnparsedFragments,
	}, nil
}

func normalizationOwner() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "localhost"
	}
	return fmt.Sprintf("%s-%d-%d", hostname, os.Getpid(), time.Now().UnixNano())
}

var _ usecase.Parser = parserAdapter{}
