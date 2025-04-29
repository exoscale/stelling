package config

import (
	"fmt"
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
