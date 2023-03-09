package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	sconfig "github.com/exoscale/stelling/config"
)

// Server is a simplified dummy HTTP server configuration
// This will typically be an embeddable configuration in another module
type Server struct {
	Endpoint string `default:"localhost"`
	Port     uint   `default:"8080"`
}

type Logging struct {
	Mode string `default:"development" validate:"oneof=development preproduction production"`
}

// Config is an example application config
// It embeds the HTTP config and supplies some validation rules
type Config struct {
	Mode         string `default:"fast" validate:"oneof=slow medium fast"`
	FeatureFlag  bool
	StartTimeout time.Duration
	Server       `json:"Server"`  // json tag added for the output logging, it is not necessary for loading config from json
	Logging      `json:"Logging"` // json tag added for the output logging, it is not necessary for loading config from json
}

// ApplyDefaults sets additional default values
// In this case it overwrites the default value for Port set by
// the tags in ServerConfig
func (c *Config) ApplyDefaults() {
	c.Server.Port = 9090
}

// In this example we'll show the order in which configuration is loaded
func Example() {
	// We overwrite the value of Config.Mode via an environment variable
	if err := os.Setenv("CONFIG_MODE", "slow"); err != nil {
		panic(err)
	}

	// We overwrite StartTimeout via a configfile
	tmpfile, err := os.CreateTemp("", "")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpfile.Name())
	_, err = tmpfile.WriteString(`{"starttimeout": "1m"}`)
	if err != nil {
		panic(err)
	}

	// We prepare custom commandline arguments to overwrite the logging mode
	osArgs := []string{"example", "-f", tmpfile.Name(), "--logging.mode", "production", "--feature-flag"}

	// This is the code you'd run in your app to load config
	// You would pass in os.Args
	conf := &Config{}
	if err := sconfig.Load(conf, osArgs); err != nil {
		panic(err)
	}

	// Printing out the config to show what it looks like
	output, err := json.Marshal(conf)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", output)

	// Output:
	// {"Mode":"slow","FeatureFlag":true,"StartTimeout":60000000000,"Server":{"Endpoint":"localhost","Port":9090},"Logging":{"Mode":"production"}}
}
