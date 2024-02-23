package multiconfig

import (
	"errors"
	"reflect"
)

// InterfaceLoader satisfies the loader interface. It recursively checks if a
// struct implements the DefaultValues interface and applies the function
// depth first, if it does
// This is useful in case you want to overwrite default values set by tags
// in a nested struct
type InterfaceLoader struct {
}

type DefaultValues interface {
	ApplyDefaults()
}

// structFields returns the exported fields of a struct value or pointer
// returns nil if the input is not a struct
func structFields(v reflect.Value) []reflect.Value {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()

	fields := []reflect.Value{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Saves us a bunch of checks later
		if !field.IsExported() {
			continue
		}
		fv := v.FieldByName(field.Name)
		fields = append(fields, fv)
	}

	return fields
}

// Load will populate s by recursively calling the `ApplyDefaults` method on it
func (l *InterfaceLoader) Load(s interface{}) error {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Pointer {
		return errors.New("cannot load into a value: target must be a pointer")
	}
	if v.IsNil() {
		return errors.New("cannot load into a nil pointer")
	}
	l.processValue(v)
	return nil
}

// processValue is the actual implementation of Load
// It was split out so that the signature is more amenable for the recursion that it does
func (l *InterfaceLoader) processValue(v reflect.Value) {
	for _, field := range structFields(v) {
		switch field.Kind() {
		case reflect.Struct:
			l.processValue(field)
		case reflect.Pointer:
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			if field.Elem().Kind() == reflect.Struct {
				l.processValue(field)
			} else {
				if applyDefaults := field.MethodByName("ApplyDefaults"); applyDefaults.IsValid() {
					applyDefaults.Call([]reflect.Value{})
				}
			}
		default:
			if field.CanAddr() {
				if applyDefaults := field.Addr().MethodByName("ApplyDefaults"); applyDefaults.IsValid() {
					applyDefaults.Call([]reflect.Value{})
				}
			}
		}
	}

	if v.Kind() == reflect.Pointer {
		if applyDefaults := v.MethodByName("ApplyDefaults"); applyDefaults.IsValid() {
			applyDefaults.Call([]reflect.Value{})
		}
	} else {
		if applyDefaults := v.Addr().MethodByName("ApplyDefaults"); applyDefaults.IsValid() {
			applyDefaults.Call([]reflect.Value{})
		}
	}
}
