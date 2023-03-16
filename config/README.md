# Exoscale Config
This package provides a convenient way to load configuration from various sources, along with validation rules for each item.

It builds upon the excellent [multiconfig](https://github.com/exoscale/multiconfig) and [validator](https://github.com/go-playground/validtor) libraries.

Since it generates a CLI, it is intended to be used by daemons and long-lived server applications. If the application needs a CLI that is meant for regular human consumption (like the `exo` commandline app), it is better to use something like urfave/cli which allows the creation of much more ergnomic terminal APIs.

## Usage
In order to use this package, all you have to do is declare a configuration struct somewhere:

```go
type Config struct {
    Endpoint string `default:"http://localhost:8080" validate:"url"`
    ApiKey   string `validate:"required"`
}
```

And then load it in your service main:

```go
conf := Config{}
if err := config.Load(&conf, os.Args); err != nil {
    // Configuration did not pass validation: inform the user and quit
    log.Fatal(err)
}
// You can rely on your config being well formed past this point
log.Info(conf)
```

There is no need to define an additional CLI.

If the path to the configuration file is an empty string, loading of a configuration file will be skipped. The location of the configuration file is determined by the `-f` or `--file` flag in `os.Args`, which is passed into the Load function.

## Validation
This package embeds the [go-playground/validator](https://github.com/go-playground/validator) library. Any validation function of this library can be used in the struct tags.

A complete list of all supported validators can be found here: https://godoc.org/github.com/go-playground/validator

The package currently doesn't expose a way to register custom validators: this is to encourage consumers to register additional validators directly here, making them available to everybody else. The following additional validators are available:

* _port_: Validates that the int value can be used as a port number
* _exoscale\_zone_: Validates that the string is a known zone short string (eg: 'gva2')
* _exoscale\_zone\_long_: Validates that the string is a known zone in cloudstack (eg: 'ch-gva-2')
* _duration_string_: Validates that the string is parseable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration) (eg: '500ms')

## Load order
This package will attempt to load configuration information from the following sources, in order:

1. Default values in struct tags
2. Default values provided by the `ApplyDefaults` method
3. YAML configuration file
4. Environment variables
5. CLI flags

Variables loaded later will override previously loaded values: thus CLI flags will override env variables, which themselves override the values found in the configuration file.

## Future improvements
* Provide a function that can safely log the config. The idea is that if a parameter is marked with a `sensitive` tag, its value will be masked in the string output.

## FAQ

### How do I create a required boolean parameter?
The `required` validator simply checks that the parameter value is not equal to its type's default value. In the case of `bool`, the default value is `false`. As a result, validation would fail if you explicitly set the parameter to `false`.

The workaround is to use a `*bool`, which default value is `nil`. The downside is that you will have to dereference it each time you need to access the value.
