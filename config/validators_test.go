package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePortNumber(t *testing.T) {
	cases := []struct {
		name     string
		input    int64
		hasError bool
	}{
		{
			name:     "Should accept value < 65535",
			input:    5335,
			hasError: false,
		},
		{
			name:     "Should reject value > 65535",
			input:    1234567,
			hasError: true,
		},
		{
			name:     "Should accept 0",
			input:    0,
			hasError: false,
		},
		{
			name:     "Should reject negative value",
			input:    -123,
			hasError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePortNumber(tc.input)
			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateExoscaleZone(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		hasError bool
	}{
		{
			name:     "Should accept a valid exoscale zone",
			input:    "gva2",
			hasError: false,
		},
		{
			name:     "Should reject the empty string",
			input:    "",
			hasError: true,
		},
		{
			name:     "Should reject any other string that does not represent a zone",
			input:    "foo",
			hasError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExoscaleZone(tc.input)
			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateExoscaleZoneLong(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		hasError bool
	}{
		{
			name:     "Should accept a valid exoscale zone",
			input:    "ch-gva-2",
			hasError: false,
		},
		{
			name:     "Should reject the empty string",
			input:    "",
			hasError: true,
		},
		{
			name:     "Should reject any other string that does not represent a zone",
			input:    "foo",
			hasError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExoscaleZoneLong(tc.input)
			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
