// Package config provides a way to load a configuration from a mix of files, env variables and cli flags and to validate it.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/exoscale/multiconfig"
	"github.com/go-playground/validator/v10"
)

// Option applies options to the validation and loading logic
type Option func(*loaderConfig)

type loaderConfig struct {
	tagLoader  *multiconfig.TagLoader
	envLoader  *multiconfig.EnvironmentLoader
	flagLoader *multiconfig.FlagLoader
	validate   *validator.Validate
}

// WithValidator replaces the built-in validator with a user supplied one
func WithValidator(validate *validator.Validate) Option {
	return func(conf *loaderConfig) {
		conf.validate = validate
	}
}

// WithLegacyFlags will change the flag format to "--struct1-struct2-myoption"
// rather than "--struct1.struct2.my-option"
// It provides backwards compatibility with the old default flag format
func WithLegacyFlags() Option {
	return func(conf *loaderConfig) {
		if conf.flagLoader == nil {
			conf.flagLoader = &multiconfig.FlagLoader{}
		}
		conf.flagLoader.CamelCase = false
		conf.flagLoader.StructSeparator = "-"
	}
}

// Load will populate s with configuration and validate it
// It will load from the following sources in order:
//  1. The `default` struct tag
//  2. The configuration file at configPath (if it is not the empty string)
//  3. Environment variables
//  4. CLI flags
//
// After loading, Load will validate the values with the functions passed into the `validate` struct tag
// If any value doesn't pass validation, a user readable error will be returned.
func Load(s interface{}, args []string, opts ...Option) error {
	// Check if --version or -v flag are passed
	if versionRequested(args[1:]) {
		if info, ok := debug.ReadBuildInfo(); ok {
			fmt.Fprintln(flag.CommandLine.Output(), formatVersion(info))
			// Asking for a version should not return an error result code
			os.Exit(0)
		}
		// If we have no support for BuildInfo, just continue as usual
	}

	// Before loading any config, we want to check if the user has provided
	// a config file path through a CLI flag
	configPath, newArgs, err := getConfigPath(args)
	if err != nil {
		return err
	}

	conf := &loaderConfig{
		// Load default configuration from struct tags
		tagLoader: &multiconfig.TagLoader{},
		// Load configuration from environment variables
		envLoader: &multiconfig.EnvironmentLoader{},
		// Load configuration from CLI flags
		flagLoader: &multiconfig.FlagLoader{
			Args:            newArgs[1:],
			CamelCase:       true,
			StructSeparator: ".",
		},
	}

	// Apply all the options
	for _, opt := range opts {
		opt(conf)
	}

	// If we didn't receive an external validator, provision one now
	if conf.validate == nil {
		conf.validate = validator.New()
	}

	var loader multiconfig.Loader
	// If a path to a configuration file is provided, add it to the chain
	if configPath != "" {
		yaml := &multiconfig.YAMLLoader{Path: configPath}
		loader = multiconfig.MultiLoader(conf.tagLoader, yaml, conf.envLoader, conf.flagLoader)
	} else {
		loader = multiconfig.MultiLoader(conf.tagLoader, conf.envLoader, conf.flagLoader)
	}

	if err := loader.Load(s); err == flag.ErrHelp {
		// Asking for help should not return an error result code
		os.Exit(0)
	} else if err != nil {
		return err
	}

	if err := registerValidators(conf.validate); err != nil {
		return err
	}

	if err := conf.validate.Struct(s); err != nil {
		// Print better error messages
		validationErrors := err.(validator.ValidationErrors)

		if len(validationErrors) > 0 {
			e := validationErrors[0]
			errorString := fmt.Sprintf(
				"Configuration error: '%s' = '%v' does not validate ",
				e.StructNamespace(),
				e.Value(),
			)
			if e.Param() == "" {
				errorString += fmt.Sprintf("'%s'", e.ActualTag())
			} else {
				errorString += fmt.Sprintf("'%s=%v'", e.ActualTag(), e.Param())
			}
			return fmt.Errorf(errorString)
		}
	}

	return nil
}

// registerValidators registers our custom validator functions on the Validate object
func registerValidators(validate *validator.Validate) error {
	validators := []struct {
		tag       string
		validator func(validator.FieldLevel) bool
	}{
		{
			tag: "port",
			validator: func(fl validator.FieldLevel) bool {
				return validatePortNumber(fl.Field().Int()) == nil
			},
		},
		{
			tag: "exoscale_zone",
			validator: func(fl validator.FieldLevel) bool {
				return validateExoscaleZone(fl.Field().String()) == nil
			},
		},
		{
			tag: "exoscale_zone_long",
			validator: func(fl validator.FieldLevel) bool {
				return validateExoscaleZoneLong(fl.Field().String()) == nil
			},
		},
	}

	for _, v := range validators {
		if err := validate.RegisterValidation(v.tag, v.validator); err != nil {
			return err
		}
	}

	return nil
}

var errMultipleFileFlag = errors.New("The file flag can be specified at most once")
var errNoConfigPathValue = errors.New("No value provided for file flag")

// getConfigPath parses the input for a `-f` flag and returns args with `-f` removed
// It will return a user readable error in case `-f` could not be parsed
// Does not modify the input
func getConfigPath(args []string) (string, []string, error) {
	newArgs := make([]string, 0, len(args))
	configPath := ""

	i := 0
	for {
		if i >= len(args) {
			break
		}
		arg := args[i]

		var argSplit []string
		if strings.HasPrefix(arg, "--") {
			argSplit = strings.SplitN(strings.TrimPrefix(arg, "--"), "=", 2)
		} else if strings.HasPrefix(arg, "-") {
			argSplit = strings.SplitN(strings.TrimPrefix(arg, "-"), "=", 2)
		} else {
			// Not a flag, move on
			newArgs = append(newArgs, arg)
			i++
			continue
		}

		// Nothing left to parse, move on
		if len(argSplit) == 0 {
			newArgs = append(newArgs, arg)
			i++
			continue
		}

		// Not the flags we're looking for, move on
		if argSplit[0] != "f" && argSplit[0] != "file" {
			newArgs = append(newArgs, arg)
			i++
			continue
		}

		// We only want the config path to be specified once
		if configPath != "" {
			return configPath, newArgs, errMultipleFileFlag
		}

		// A `-f /my/path` style has been used: process the next argument to
		// determine the value
		if len(argSplit) == 1 {
			i++
			if i >= len(args) {
				return configPath, newArgs, errNoConfigPathValue
			}
			flagValue := args[i]

			if strings.HasPrefix(flagValue, "--") || strings.HasPrefix(flagValue, "-") {
				return configPath, newArgs, errNoConfigPathValue
			}

			configPath = flagValue
			i++
			continue

		}

		// len(argSplit) == 2: Pick the value after the '='
		configPath = argSplit[1]
		i++
		continue
	}

	return configPath, newArgs, nil
}

// versionRequested will return true if the args contain the special --version or -v flags
func versionRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-v" {
			return true
		}
	}
	return false
}

func formatVersion(info *debug.BuildInfo) string {
	main := info.Path
	version := info.Main.Version
	goVersion := info.GoVersion

	revision := "unknown"
	revisionTimestamp := "unknown"
	dirty := ""
	buildFlags := []string{}

	for _, item := range info.Settings {
		switch item.Key {
		case "vcs.revision":
			revision = item.Value
		case "vcs.time":
			revisionTimestamp = item.Value
		case "vcs.modified":
			if item.Value == "true" {
				dirty = " (dirty)"
			}
		case "CGO_ENABLED",
			"CGO_FLAGS",
			"CGO_CFLAGS",
			"CGO_CPPFLAGS",
			"CGO_CXXFLAGS",
			"CGO_LDFLAGS":
			buildFlags = append(buildFlags, fmt.Sprintf("%s: \"%s\"", item.Key, item.Value))
		}
	}

	// TODO: maybe use the template engine?
	format := `%s %s on %s
  Revision %s%s
  Committed at %s

Build Flags:
  %s`
	return fmt.Sprintf(format, main, version, goVersion, revision, dirty, revisionTimestamp, strings.Join(buildFlags, "\n  "))
}
