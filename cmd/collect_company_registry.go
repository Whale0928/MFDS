package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

type RunCompanyRegistryCollectionFunc func(
	context.Context,
	config.Config,
	CompanyRegistrySyncCommand,
) (companyregistry.Summary, error)

type CompanyRegistrySyncCommand struct {
	Since string
}

func newSyncCompanyRegistryCommand(
	getConfig func() config.Config,
	run RunCompanyRegistryCollectionFunc,
	out io.Writer,
) *cobra.Command {
	var since string
	command := &cobra.Command{
		Use:   "sync-company-registry",
		Short: "수입업체 공식정보 변경분을 동기화합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			summary, err := run(cmd.Context(), getConfig(), CompanyRegistrySyncCommand{Since: since})
			if err != nil {
				return err
			}
			fmt.Fprintf(out,
				"sync-company-registry 완료: sync_id=%d since=%s through=%s services=%d business_license_fetches=%d business_license_rows=%d closure_fetches=%d closure_rows=%d excellent_importer_fetches=%d excellent_importer_rows=%d disposition_fetches=%d disposition_rows=%d\n",
				summary.CollectionID, summary.Since.Format("2006-01-02"), summary.Through.Format("2006-01-02"),
				len(summary.Services),
				summary.Services[companyregistry.ServiceC001].Fetches, summary.Services[companyregistry.ServiceC001].Rows,
				summary.Services[companyregistry.ServiceI2821].Fetches, summary.Services[companyregistry.ServiceI2821].Rows,
				summary.Services[companyregistry.ServiceI0250].Fetches, summary.Services[companyregistry.ServiceI0250].Rows,
				summary.Services[companyregistry.ServiceI0470].Fetches, summary.Services[companyregistry.ServiceI0470].Rows,
			)
			return nil
		},
	}
	command.Flags().StringVar(&since, "since", "", "첫 동기화 변경일자(YYYY-MM-DD, 이후에는 마지막 성공일 자동 사용)")
	return command
}
