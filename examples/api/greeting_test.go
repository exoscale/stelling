package api_test

import (
	"net/http"
	"testing"

	"github.com/exoscale/stelling/examples/api"
	"github.com/exoscale/stelling/examples/api/public"
	"github.com/exoscale/stelling/examples/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestGetStellingGreeting(t *testing.T) {
	tests := []struct {
		name     string
		greeting string
		expected public.GetStellingGreetingResponseObject
	}{
		{
			name:     "empty greeting config returns empty greeting",
			expected: public.GetStellingGreeting200JSONResponse{},
		},
		{
			name:     "non-empty greeting config returns greeting",
			greeting: "blah blah",
			expected: public.GetStellingGreeting200JSONResponse{
				Message: "blah blah",
			},
		},
		{
			name:     "error response",
			greeting: "you are forbidden",
			expected: public.GetStellingGreeting403JSONResponse{
				Title:  "access forbidden",
				Detail: "greeting contains the word forbidden",
				Status: http.StatusForbidden,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &config.Config{
				Greeting: config.Greeting{
					Message: tt.greeting,
				},
			}

			server := api.NewAPIServer(conf, zaptest.NewLogger(t))

			response, err := server.GetStellingGreeting(t.Context(), public.GetStellingGreetingRequestObject{})
			require.NoError(t, err)
			require.Equal(t, tt.expected, response)
		})
	}
}
