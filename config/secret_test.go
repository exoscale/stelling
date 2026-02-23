package config

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSecret(t *testing.T) {
	t.Run("Should log *****", func(t *testing.T) {
		expected := "my secret *****\n"
		input := Secret("mystery")
		actual := fmt.Sprintln("my secret", input)
		require.Equal(t, expected, actual)

		expected = "my secret ***** is hidden"
		actual = fmt.Sprintf("my secret %s is hidden", input)
		require.Equal(t, expected, actual)
	})

	t.Run("Should return its plaintext", func(t *testing.T) {
		input := Secret("mystery")
		expected := "mystery"
		require.Equal(t, expected, input.Reveal())
	})

	t.Run("Should return ***** as string value", func(t *testing.T) {
		input := Secret("mystery")
		expected := "*****"
		require.Equal(t, expected, input.String())
	})

	t.Run("Should decode from json correctly", func(t *testing.T) {
		type Config struct {
			MySecret Secret
		}
		var config Config
		input := `{"MySecret": "mystery"}`
		require.NoError(t, json.Unmarshal([]byte(input), &config))
		expected := Config{MySecret: "mystery"}
		require.Equal(t, expected, config)
	})

	t.Run("Should encode to json as ****", func(t *testing.T) {
		input := struct {
			MySecret Secret
		}{
			MySecret: "mystery",
		}
		output, err := json.Marshal(input)
		require.NoError(t, err)
		expected := `{"MySecret":"*****"}`
		require.Equal(t, []byte(expected), output)
	})

	t.Run("Should decode from yaml correctly", func(t *testing.T) {
		type Config struct {
			MySecret Secret
		}
		var config Config
		input := `mysecret: mystery`
		require.NoError(t, yaml.Unmarshal([]byte(input), &config))
		expected := Config{MySecret: "mystery"}
		require.Equal(t, expected, config)
	})

	t.Run("Should encode to yaml as ****", func(t *testing.T) {
		input := struct {
			MySecret Secret
		}{
			MySecret: "mystery",
		}
		output, err := yaml.Marshal(input)
		require.NoError(t, err)
		expected := `mysecret: '*****'
`
		require.Equal(t, []byte(expected), output)
	})
}
