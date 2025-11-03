package utils

import (
	"flag"
	"os"
	"strconv"

	"github.com/securesign/operator/internal/images"
)

// StringFlagOrEnv defines a string flag which can be set by an environment variable.
// Precedence: flag > env var > default value.
func StringFlagOrEnv(p *string, name string, envName string, defaultValue string, usage string) {
	envValue := os.Getenv(envName)
	if envValue != "" {
		defaultValue = envValue
	}
	flag.StringVar(p, name, defaultValue, usage)
}

func RelatedImageFlag(name string, image images.Image, usage string) {
	p := new(string)
	StringFlagOrEnv(p, name, string(image), images.Registry.Get(image), usage)
	images.Registry.Set(image, *p)
}

// BoolFlagOrEnv defines a bool flag which can be set by an environment variable.
// Precedence: flag > env var > default value.
func BoolFlagOrEnv(p *bool, name string, envName string, defaultValue bool, usage string) {
	envValue := os.Getenv(envName)
	if envName != "" {
		defaultValue, _ = strconv.ParseBool(envValue)
	}
	flag.BoolVar(p, name, defaultValue, usage)
}

// IsFlagProvided checks if a flag was explicitly provided on the command line or via environment variable
func IsFlagProvided(name string, envName string) bool {
	if envName != "" && os.Getenv(envName) != "" {
		return true
	}

	provided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}
