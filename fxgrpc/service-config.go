package fxgrpc

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap/zapcore"
)

type ServiceConfig struct {
	LoadBalancingPolicy string         `validate:"omitempty,oneof=pick_first round_robin"`
	MethodConfig        []MethodConfig ``
}

func (c *ServiceConfig) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("load-balancing-policy", c.LoadBalancingPolicy)
	if err := enc.AddReflected("method-config", &c.MethodConfig); err != nil {
		return err
	}

	return nil
}

func (c ServiceConfig) MarshalJSON() ([]byte, error) {
	result := map[string][]byte{}

	fmt.Println("foobar", c)

	if len(c.LoadBalancingPolicy) != 0 {
		value, err := json.Marshal(c.LoadBalancingPolicy)
		if err != nil {
			return nil, err
		}
		result["loadBalancingConfig"] = value
	}

	if len(c.MethodConfig) != 0 {
		value, err := json.Marshal(c.MethodConfig)
		if err != nil {
			return nil, err
		}
		result["methodConfig"] = value
	}

	return json.Marshal(result)
}

// Just to make sure that interfaces are correctly implemented
func EnsureJsonInterfaceImplemented() {
	var _ json.Marshaler = (*ServiceConfig)(nil)
}

type MethodConfig struct {
	// Name is a list of methods to match with this config
	Name []MethodName

	// WaitForReady bool // Side effect of this is too dangerous

	// Timeout for this methods, string representing a duration for json/grpc encoding
	Timeout string

	// I didn't found a way to have nilable uint32
	// MaxRequestMessageBytes  *uint32
	// MaxResponseMessageBytes *uint32

	RetryPolicy RetryPolicy
}

func (c *MethodConfig) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if err := enc.AddReflected("name", &c.Name); err != nil {
		return err
	}
	enc.AddString("timeout", c.Timeout)
	if err := enc.AddObject("retry-policy", &c.RetryPolicy); err != nil {
		return nil
	}

	return nil
}

type MethodName struct {
	Service string `validate:"required"`
	Method  string
}

func (c *MethodName) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("service", c.Service)
	enc.AddString("method", c.Method)
	return nil
}

type RetryPolicy struct {
	// MaxAttempts is the maximum number of RPC attempts, including the original attempt.
	MaxAttempts uint32 `validate:"required,gt=1"`

	InitialBackoff    string  `validate:"required,notblank"`
	MaxBackoff        string  `validate:"required,notblank"`
	BackoffMultiplier float64 `validate:"required,gt=0"`

	RetryableStatusCodes []string `validate:"required,notblank"`
}

func (c *RetryPolicy) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint32("max-attempts", c.MaxAttempts)
	enc.AddString("initial-backoff", c.InitialBackoff)
	enc.AddString("max-backoff", c.MaxBackoff)
	enc.AddFloat64("backoff-multiplier", c.BackoffMultiplier)

	if err := enc.AddReflected("retryable-status-codes", c.RetryableStatusCodes); err != nil {
		return nil
	}

	return nil
}
