package config

import (
	"fmt"
	"slices"
)

func validatePortNumber(input int64) error {
	if input < 0 {
		return fmt.Errorf("port numbers cannot be negative. Received: %d", input)
	}
	if input > 65535 {
		return fmt.Errorf("port numbers cannot be larger than 65535. Received: %d", input)
	}
	return nil
}

func validateExoscaleZone(input string) error {
	// The list of short zone strings can be found here:
	// https://github.com/exoscale/puppet/blob/master/configstore/common.yaml#L1114
	// It is also referred to as "location" in puppet variables
	zones := []string{
		"zrh1",
		"fra1",
		"gva2",
		"muc1",
		"sof1",
		"vie1",
		"vie2",
		"zrh1",
		"aws",
	}

	if slices.Contains(zones, input) {
		return nil
	}

	return fmt.Errorf("'%s' is not a valid Exoscale zone", input)
}

func validateExoscaleZoneLong(input string) error {
	// The master for the zones long string is cloudstack
	// An up to date list can always be found here:
	// https://github.com/exoscale/puppet/blob/master/configstore/platform/prod-portal-front.yaml#L201
	// This list does not update often enough to justify producing it with go generate
	zones := []string{
		"ch-gva-2",
		"de-fra-1",
		"de-muc-1",
		"ch-dk-2",
		"at-vie-1",
		"at-vie-2",
		"bg-sof-1",
	}

	if slices.Contains(zones, input) {
		return nil
	}

	return fmt.Errorf("'%s' is not a valid Exoscale zone", input)
}
