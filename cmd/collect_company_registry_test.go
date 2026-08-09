package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

func TestCollectCompanyRegistry_수집성공_서비스별수치와매칭결과를출력한다(t *testing.T) {
	output := &strings.Builder{}
	command := newCollectCompanyRegistryCommand(
		func() config.Config { return config.Config{} },
		func(context.Context, config.Config) (companyregistry.Summary, error) {
			return companyregistry.Summary{
				CollectionID: 51,
				Services: map[companyregistry.ServiceID]companyregistry.ServiceSummary{
					companyregistry.ServiceC001:  {Fetches: 82, Rows: 81805},
					companyregistry.ServiceI2821: {Fetches: 31, Rows: 30001},
					companyregistry.ServiceI0250: {Fetches: 1, Rows: 59},
					companyregistry.ServiceI0470: {Fetches: 6, Rows: 5452},
				},
				Matches: map[companyregistry.MatchStatus]uint64{
					companyregistry.MatchExactName: 320, companyregistry.MatchNormalizedName: 55,
					companyregistry.MatchAmbiguous: 10, companyregistry.MatchUnresolved: 11,
				},
			}, nil
		},
		output,
	)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"collection_id=51", "C001_rows=81805", "I0470_rows=5452", "exact=320", "unresolved=11"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("output = %q, missing %q", output.String(), value)
		}
	}
}
