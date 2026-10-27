package openapi

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// RequestFields returns a field error for each problem of err, a request validation, decoding or binding error
// The field errors name the fields as the client sent them, eg "rules[0].action", and never quote their values
// An error which does not point to a field gives a single field error about the whole request
func RequestFields(err error) []apierror.Field {
	var fields []apierror.Field
	collectFields(err, "", "", &fields)
	return fields
}

func collectFields(err error, in, name string, fields *[]apierror.Field) {
	switch e := err.(type) {
	case openapi3.MultiError:
		for _, err := range e {
			collectFields(err, in, name, fields)
		}
		return
	case *openapi3filter.RequestError:
		if e.Parameter != nil {
			in, name = e.Parameter.In, e.Parameter.Name
		}
		if e.Err != nil {
			collectFields(e.Err, in, name, fields)
			return
		}
	case *openapi3.SchemaError:
		path := e.JSONPointer()
		if e.SchemaField == "properties" {
			// shortcut: kin-openapi only names an unknown property in its reason, parse it until it marks its path
			var property string
			if _, err := fmt.Sscanf(e.Reason, "property %q is unsupported", &property); err == nil {
				path = append(path, property)
			}
		}
		*fields = append(*fields, field(in, fieldName(name, path), schemaCode(e.SchemaField)))
		return
	}

	code := apierror.CodeInvalidData
	if _, ok := errors.AsType[*openapi3filter.ParseError](err); ok {
		code = apierror.CodeInvalidFormat
	} else if errors.Is(err, openapi3filter.ErrInvalidRequired) || errors.Is(err, openapi3filter.ErrInvalidEmptyValue) {
		code = apierror.CodeRequired
	}
	*fields = append(*fields, field(in, name, code))
}

func field(in, name string, code apierror.Code) apierror.Field {
	if in == "" {
		return apierror.F(name, code)
	}
	return apierror.FIn(in, name, code)
}

// fieldName joins the name of a parameter and the path of a value inside it, eg rules[0].action
func fieldName(name string, path []string) string {
	for _, segment := range path {
		if _, err := strconv.Atoi(segment); err == nil {
			name += "[" + segment + "]"
		} else if name == "" {
			name = segment
		} else {
			name += "." + segment
		}
	}
	return name
}

// schemaCode returns the code of a failure of the given schema keyword
func schemaCode(keyword string) apierror.Code {
	switch keyword {
	case "required":
		return apierror.CodeRequired
	case "enum", "const":
		return apierror.CodeUnknownValue
	case "type", "format", "pattern":
		return apierror.CodeInvalidFormat
	case "minLength", "minimum", "exclusiveMinimum", "minItems", "minProperties":
		return apierror.CodeTooSmall
	case "maxLength":
		return apierror.CodeTooLong
	case "maximum", "exclusiveMaximum", "maxItems", "maxProperties":
		return apierror.CodeTooLarge
	case "properties":
		return apierror.CodeUnknownField
	default:
		return apierror.CodeInvalidData
	}
}
