package config

import (
	"os"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

var mockArgs []string = []string{"conf"}

func TestConfigDefaultValues(t *testing.T) {
	type Config struct {
		MyString string   `default:"MyString"`
		MyBool   bool     `default:"true"`
		MyInt    int      `default:"9001"`
		MyArray  []string `default:"a,b,c"`
	}

	expected := Config{
		MyString: "MyString",
		MyBool:   true,
		MyInt:    9001,
		MyArray:  []string{"a", "b", "c"},
	}

	config := Config{}
	if assert.NoError(t, Load(&config, mockArgs)) {
		assert.Equal(t, expected, config)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("Invalid configuration should return an error", func(t *testing.T) {
		type Config struct {
			MyIP string `default:"notanip" validate:"ipv4"`
		}

		config := Config{}
		assert.Error(t, Load(&config, mockArgs))
	})

	t.Run("Should populate the config if it passes validation", func(t *testing.T) {
		type Config struct {
			MyIP string `default:"0.0.0.0" validate:"ipv4"`
		}

		expected := Config{
			MyIP: "0.0.0.0",
		}

		config := Config{}
		if assert.NoError(t, Load(&config, mockArgs)) {
			assert.Equal(t, expected, config)
		}
	})
}

func TestConfigFromYAML(t *testing.T) {
	type Config struct {
		MyString string `default:"MyString"`
	}

	expected := Config{
		MyString: "YAMLVariable",
	}

	confFile, err := os.CreateTemp("", "config")
	assert.NoError(t, err, "Failed to create temporary file")
	defer os.Remove(confFile.Name())

	_, err = confFile.WriteString("mystring: YAMLVariable")
	assert.NoError(t, err, "Failed to write to temporary file")
	assert.NoError(t, confFile.Close(), "Failed to close temporary file")

	config := Config{}
	if assert.NoError(t, Load(&config, []string{"conf", "-f", confFile.Name()})) {
		assert.Equal(t, expected, config)
	}
}

func TestConfigFromEnvironment(t *testing.T) {
	type Config struct {
		MyString string `default:"MyString"`
	}

	expected := Config{
		MyString: "EnvironmentVariable",
	}

	t.Setenv("CONFIG_MY_STRING", "EnvironmentVariable")

	config := Config{}
	if assert.NoError(t, Load(&config, mockArgs)) {
		assert.Equal(t, expected, config)
	}
}

// TestConfigLoadOrder validates that the following priority is observed:
// default -> config file -> environment variable -> flag
func TestConfigLoadOrder(t *testing.T) {
	type Config struct {
		LoadDefault string `default:"Default"`
		LoadYAML    string `default:"Default"`
		LoadEnv     string `default:"Default"`
		LoadFlag    string `default:"Default"`
	}

	expected := Config{
		LoadDefault: "Default",
		LoadYAML:    "YAML",
		LoadEnv:     "Env",
		LoadFlag:    "Flag",
	}

	confFile, err := os.CreateTemp("", "config")
	assert.NoError(t, err, "Failed to create temporary file")
	defer os.Remove(confFile.Name())

	_, err = confFile.WriteString(`loadyaml: YAML
loadenv: YAML
loadflag: YAML`)
	assert.NoError(t, err, "Failed to write to temporary file")
	assert.NoError(t, confFile.Close(), "Failed to close temporary file")

	t.Setenv("CONFIG_LOAD_ENV", "Env")
	t.Setenv("CONFIG_LOAD_FLAG", "Env")

	args := []string{
		"conf",
		"-f", confFile.Name(),
		"-load-flag", "Flag",
	}

	config := Config{}
	if assert.NoError(t, Load(&config, args)) {
		assert.Equal(t, expected, config)
	}
}

func TestNestedValues(t *testing.T) {
	type Config struct {
		LoadYAML string `default:"Default"`
		Nested   struct {
			LoadDefault string `default:"Default"`
			LoadFlag    string
		}
	}

	expected := Config{
		LoadYAML: "YAML",
		Nested: struct {
			LoadDefault string `default:"Default"`
			LoadFlag    string
		}{
			LoadDefault: "Default",
			LoadFlag:    "Flag",
		},
	}

	confFile, err := os.CreateTemp("", "config")
	assert.NoError(t, err, "Failed to create temporary file")
	defer os.Remove(confFile.Name())

	_, err = confFile.WriteString(`loadyaml: YAML`)
	assert.NoError(t, err, "Failed to write to temporary file")
	assert.NoError(t, confFile.Close(), "Failed to close temporary file")

	args := []string{
		"conf",
		"-f", confFile.Name(),
		"--nested.load-flag", "Flag",
	}

	config := Config{}
	if assert.NoError(t, Load(&config, args)) {
		assert.Equal(t, expected, config)
	}
}

// TestGetConfigPath asserts that we correctly preprocess the config file flag
func TestGetConfigPath(t *testing.T) {
	cases := []struct {
		name     string
		input    []string
		expected string
		newArgs  []string
		hasError bool
		errValue error
	}{
		{
			name:     "Should handle an empty input",
			input:    []string{},
			expected: "",
			newArgs:  []string{},
			hasError: false,
		},
		{
			name:     "Should handle a nil input",
			input:    nil,
			expected: "",
			newArgs:  []string{},
			hasError: false,
		},
		{
			name:     "Should pass on input with only the command name",
			input:    []string{"conf"},
			expected: "",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should pass on input no flags",
			input:    []string{"conf", "some-input", "file"},
			expected: "",
			newArgs:  []string{"conf", "some-input", "file"},
			hasError: false,
		},
		{
			name:     "Should pass on input with no config file flag",
			input:    []string{"conf", "-flag", "-other-flag", "true"},
			expected: "",
			newArgs:  []string{"conf", "-flag", "-other-flag", "true"},
			hasError: false,
		},
		{
			name:     "Should return the configpath with a -f flag",
			input:    []string{"conf", "-f", "/config/path"},
			expected: "/config/path",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should return the configpath with a --f flag",
			input:    []string{"conf", "--f", "/config/path"},
			expected: "/config/path",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should return the configpath with a -file flag",
			input:    []string{"conf", "-file", "/config/path"},
			expected: "/config/path",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should return the configpath with a --file flag",
			input:    []string{"conf", "--file", "/config/path"},
			expected: "/config/path",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should return the configpath with -f=path style flags",
			input:    []string{"conf", "-f=/config/path"},
			expected: "/config/path",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should return the configpath even if it contains '='",
			input:    []string{"conf", "--file=/config=path/foo"},
			expected: "/config=path/foo",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should return other flags in correct order if configpath is found",
			input:    []string{"conf", "-myflag", "foo", "-f", "/config/path", "-boolflag", "argument"},
			expected: "/config/path",
			newArgs:  []string{"conf", "-myflag", "foo", "-boolflag", "argument"},
			hasError: false,
		},
		{
			name:     "Should return other flags even if they start with file when a configpath is found",
			input:    []string{"conf", "--filepath", "/other/path", "--file", "/config/path"},
			expected: "/config/path",
			newArgs:  []string{"conf", "--filepath", "/other/path"},
			hasError: false,
		},
		{
			name:     "Should return an error if the file flag is set multiple times",
			input:    []string{"conf", "-f", "/config/path", "--file", "/other/path"},
			hasError: true,
			errValue: errMultipleFileFlag,
		},
		{
			name:     "Should return an error if no value is provided for config path",
			input:    []string{"conf", "-f"},
			hasError: true,
			errValue: errNoConfigPathValue,
		},
		{
			name:     "Should return an error if no value is provided for configpath (other flags)",
			input:    []string{"conf", "-f", "--other-flag"},
			hasError: true,
			errValue: errNoConfigPathValue,
		},
		{
			name:     "Should allow an empty configpath with = equals syntax",
			input:    []string{"conf", "-f="},
			expected: "",
			newArgs:  []string{"conf"},
			hasError: false,
		},
		{
			name:     "Should handle '--' as an argument",
			input:    []string{"conf", "--boolflag", "--", "foo"},
			expected: "",
			newArgs:  []string{"conf", "--boolflag", "--", "foo"},
			hasError: false,
		},
		{
			name:     "Should handle '--' when a configpath is given",
			input:    []string{"conf", "-f", "/config/path", "--", "foo"},
			expected: "/config/path",
			newArgs:  []string{"conf", "--", "foo"},
			hasError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configPath, newArgs, err := getConfigPath(tc.input)

			if tc.hasError {
				assert.EqualError(t, err, tc.errValue.Error())
			} else {
				if assert.NoError(t, err) {
					assert.Equal(t, tc.expected, configPath)
					assert.Equal(t, tc.newArgs, newArgs)
				}
			}
		})
	}
}

func TestVersionRequested(t *testing.T) {
	cases := []struct {
		name     string
		input    []string
		expected bool
	}{
		{
			name:     "Should return false if no version is requested",
			input:    []string{"--enabled", "-h"},
			expected: false,
		},
		{
			name:     "Should return false if args is empty",
			input:    []string{},
			expected: false,
		},
		{
			name:     "Should return true if -v is in the arguments",
			input:    []string{"--help", "-v"},
			expected: true,
		},
		{
			name:     "Should return true if --version in the arguments",
			input:    []string{"--version"},
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, versionRequested(tc.input))
		})
	}
}

func TestWithValidator(t *testing.T) {
	conf := &loaderConfig{}
	validate := validator.New()
	opt := WithValidator(validate)
	opt(conf)
	assert.Equal(t, validate, conf.validate)
}

func TestWithLegacyFlags(t *testing.T) {
	conf := &loaderConfig{}
	opt := WithLegacyFlags()
	opt(conf)
	assert.False(t, conf.flagLoader.CamelCase)
	assert.Equal(t, "-", conf.flagLoader.StructSeparator)
}

func TestLoadWithOptions(t *testing.T) {
	t.Run("WithValidator", func(t *testing.T) {
		validate := validator.New()
		validate.RegisterAlias("my-alias", "ipv4")
		opt := WithValidator(validate)

		type Config struct {
			MyIP string `default:"0.0.0.0" validate:"my-alias"`
		}

		expected := Config{
			MyIP: "0.0.0.0",
		}

		config := Config{}
		if assert.NoError(t, Load(&config, mockArgs, opt)) {
			assert.Equal(t, expected, config)
		}
	})

	t.Run("WithLegacyFlags", func(t *testing.T) {
		type NestedConfig struct {
			MyIP string `default:"0.0.0.0"`
		}
		type Config struct {
			NestedConfig NestedConfig
		}

		args := []string{"testapp", "--nestedconfig-myip", "1.1.1.1"}

		expected := Config{
			NestedConfig: NestedConfig{MyIP: "1.1.1.1"},
		}

		config := Config{}
		if assert.NoError(t, Load(&config, args, WithLegacyFlags())) {
			assert.Equal(t, expected, config)
		}
	})
}
