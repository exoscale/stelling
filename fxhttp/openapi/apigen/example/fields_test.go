package example

import (
	"testing"

	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/stretchr/testify/require"
)

func TestFields(t *testing.T) {
	t.Run("Should name fields as the client sends them", func(t *testing.T) {
		for want, name := range map[string]apierror.FieldNamer{
			"":                               Fields,
			"name":                           Fields.Name,
			"destination.id":                 Fields.Destination.Id,
			"historical-versions[2].version": Fields.HistoricalVersions.At(2).Version,
			"tags[0]":                        Fields.Tags.At(0),
			"labels.env":                     Fields.Labels.Key("env"),
			"source.key-id":                  Fields.Source.KeyId,
		} {
			require.Equal(t, apierror.F(want, apierror.CodeInvalidData), apierror.FieldOf(name, apierror.CodeInvalidData))
		}
	})

	t.Run("Should say where a parameter is sent", func(t *testing.T) {
		require.Equal(t, apierror.FIn("path", "id", apierror.CodeInvalidFormat), apierror.FieldOf(Fields.Id, apierror.CodeInvalidFormat))
		require.Equal(t, apierror.FIn("query", "dry-run", apierror.CodeInvalidFormat), apierror.FieldOf(Fields.DryRun, apierror.CodeInvalidFormat))
	})
}
