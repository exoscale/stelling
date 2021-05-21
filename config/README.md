# Exoscale Config
This package provides a conventient way to load configuration from various sources, along with validation rules for each item.

It builds upon the excellent [multiconfig](https://github.com/koding/multiconfig) and [validator](https://github.com/go-playground/validtor) libraries.

Since it generates a CLI, it is intended to be used by daemons and long lived server applications. If the application needs a CLI that is meant for regular human consumption (like the `exo` commandline app), it is better to use something like urfave/cli which allows the creation much more ergnomic terminal APIs.

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

If the path to the configuration file is the empty string, loading of a configuration file will be skipped. The location of the configuration file is determined by the `-f` or `--file` flag in `os.Args`, which is passed into the Load function.

## Validation
This package embeds the [go-playground/validator](https://github.com/go-playground/validator) library. Any validation function of this library can be used in the struct tags.

A complete list of all supported validators can be found here: https://godoc.org/github.com/go-playground/validator

The package currently doesn't expose a way to register custom validators: this is to encourage consumers to register additional validators directly here, making them available to everybody else. The following additional validators are available:

* _port_: Validates that the int value can be used as a port number
* _exoscale\_zone_: Validates that the string is a known zone short string (eg: 'gva2')
* _exoscale\_zone\_long_: Validates that the string is a known zone in cloudstack (eg: 'ch-gva-2')

## Load order
It will load the following sources in order:

1. Default values in struct tags
2. YAML configuration file
3. Environment variables
4. CLI flags

Variables loaded later will overwrite earlier loaded variables: cli flags will overwite env variables, and env variables overwrite the configuration file.

## Future improvement
* Provide a function that can safely log the config. The idea is that if a parameter is marked with a `sensitive` tag, it will be masked in the string output.

## FAQ

### How do I make a boolean required
The `required` validator checks whether the parameter does not have it's type default value. For bool's, the default value is `false`. The end result is that validation will fail if you explicitly set the parameter to `false`.

The workaround is to use a `*bool`, who's default value is `nil`. The downside is that you will have to dereference each time you want to read the value.