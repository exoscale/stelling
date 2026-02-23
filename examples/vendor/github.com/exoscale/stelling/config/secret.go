package config

import "fmt"

// Secret is a unitily type that behaves exactly like a string but will produce **** when serialized
// This prevents the value from accidentally being logged or being transmitted in the clear
// The underlying value can be accessed via its Plaintext method
type Secret string

func (s *Secret) UnmarshalText(text []byte) error {
	*s = Secret(string(text))
	return nil
}

func (s Secret) MarshalText() ([]byte, error) {
	if len(s) == 0 {
		return []byte{}, nil
	}
	return []byte("*****"), nil
}

func (s Secret) String() string {
	if len(s) == 0 {
		return ""
	}
	return "*****"
}

func (s Secret) GoString() string {
	if len(s) == 0 {
		return ""
	}
	return "*****"
}

func (s Secret) Format(state fmt.State, verb rune) {
	width, ok := state.Width()
	if !ok {
		width = 5
	}
	var oChar = byte('*')
	if len(s) == 0 {
		oChar = byte(' ')
	}
	output := make([]byte, width)
	for i := range output {
		output[i] = oChar
	}
	_, _ = state.Write(output)
}

func (s *Secret) Plaintext() string {
	return string(*s)
}
