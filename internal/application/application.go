package application

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ApplicationConfig struct {
	SingleInstance bool `json:"single_instance"`
	LastUpdated    string
	Version        string
	Variables      map[string]appVariable `json:"variables"`
}

type appVariable struct {
	Type        string          `json:"type"`
	Required    bool            `json:"required"`
	Default     json.RawMessage `json:"default"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description"`
	InstallOnly bool            `json:"install_only"`
}

func LoadConfig(path string) (ApplicationConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ApplicationConfig{}, err
	}

	var config ApplicationConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ApplicationConfig{}, err
	}
	return config, nil
}

func isEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return true
	}

	var value string
	return json.Unmarshal(raw, &value) == nil &&
		strings.TrimSpace(value) == ""
}

func decodeValue(raw json.RawMessage, kind string) (any, error) {
	switch kind {
	case "int", "port":
		var value int
		err := json.Unmarshal(raw, &value)
		return value, err

	case "float":
		var value float64
		err := json.Unmarshal(raw, &value)
		return value, err

	case "string":
		var value string
		err := json.Unmarshal(raw, &value)
		return value, err

	case "bool":
		var value bool
		err := json.Unmarshal(raw, &value)
		return value, err

	default:
		return nil, fmt.Errorf("unsupported type %q", kind)
	}
}

// this converts JSON data passed by a client into a map[string]string
// for passing this as ENV variables
func ValidateParamsAgainstConfig(
	data map[string]json.RawMessage,
	config ApplicationConfig,
) (map[string]string, map[string]string, error) {
	commonVars := make(map[string]string, len(config.Variables))
	installOnlyVars := make(map[string]string, len(config.Variables))

	for name, field := range config.Variables {
		raw := data[name]

		if isEmpty(raw) {
			switch {
			case field.Default != nil:
				raw = field.Default
				if isEmpty(raw) {
					return nil, nil, fmt.Errorf("%s: default cannot be empty", name)
				}
			case field.Required:
				return nil, nil, fmt.Errorf("%s: required", name)
			default:
				continue
			}
		}

		value, err := decodeValue(raw, field.Type)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"%s: expected %s: %w", name, field.Type, err,
			)
		}

		if field.InstallOnly {
			installOnlyVars[name] = fmt.Sprint(value)
		} else {
			commonVars[name] = fmt.Sprint(value)
		}
	}

	return commonVars, installOnlyVars, nil
}

var PrivateAppIDS = []string{
	"dawa",
	"metrics",
	"caddy",
}

var AppIDs = []string{
	"synapse",
	"cryptpad",
	"peertube",
	"mastodon",
	"ersatz",
}
