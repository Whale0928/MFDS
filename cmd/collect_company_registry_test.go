package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

func TestSyncCompanyRegistry_동기화성공_서비스별수치와매칭결과를출력한다(t *testing.T) {
	output := &strings.Builder{}
	command := newSyncCompanyRegistryCommand(
		func() config.Config { return config.Config{} },
		func(_ context.Context, _ config.Config, command CompanyRegistrySyncCommand) (companyregistry.Summary, error) {
			if command.Since != "2026-08-01" {
				t.Fatalf("Since = %q", command.Since)
			}
			return companyregistry.Summary{
				CollectionID: 51,
				Since:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				Through:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
				Services: map[companyregistry.ServiceID]companyregistry.ServiceSummary{
					companyregistry.ServiceC001:  {Fetches: 82, Rows: 81805},
					companyregistry.ServiceI2821: {Fetches: 31, Rows: 30001},
					companyregistry.ServiceI0250: {Fetches: 1, Rows: 59},
					companyregistry.ServiceI0470: {Fetches: 6, Rows: 5452},
				},
			}, nil
		},
		output,
	)
	command.SetArgs([]string{"--since", "2026-08-01"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"sync_id=51", "since=2026-08-01", "through=2026-08-10", "business_license_rows=81805", "disposition_rows=5452"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("output = %q, missing %q", output.String(), value)
		}
	}
}
