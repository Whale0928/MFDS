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
) (companyregistry.Summary, error)

func newCollectCompanyRegistryCommand(
	getConfig func() config.Config,
	run RunCompanyRegistryCollectionFunc,
	out io.Writer,
) *cobra.Command {
	return &cobra.Command{
		Use:   "collect-company-registry",
		Short: "수입업체 인허가·폐업·우수업소·행정처분 원장을 수집합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			summary, err := run(cmd.Context(), getConfig())
			if err != nil {
				return err
			}
			fmt.Fprintf(out,
				"collect-company-registry 완료: collection_id=%d services=%d C001_fetches=%d C001_rows=%d I2821_fetches=%d I2821_rows=%d I0250_fetches=%d I0250_rows=%d I0470_fetches=%d I0470_rows=%d exact=%d normalized=%d ambiguous=%d unresolved=%d\n",
				summary.CollectionID, len(summary.Services),
				summary.Services[companyregistry.ServiceC001].Fetches, summary.Services[companyregistry.ServiceC001].Rows,
				summary.Services[companyregistry.ServiceI2821].Fetches, summary.Services[companyregistry.ServiceI2821].Rows,
				summary.Services[companyregistry.ServiceI0250].Fetches, summary.Services[companyregistry.ServiceI0250].Rows,
				summary.Services[companyregistry.ServiceI0470].Fetches, summary.Services[companyregistry.ServiceI0470].Rows,
				summary.Matches[companyregistry.MatchExactName], summary.Matches[companyregistry.MatchNormalizedName],
				summary.Matches[companyregistry.MatchAmbiguous], summary.Matches[companyregistry.MatchUnresolved],
			)
			return nil
		},
	}
}
