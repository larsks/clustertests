package testutil

import (
	"fmt"
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
)

// GetEnvWithDefault is a typed version of os.Getenv that allows you to
// specify a default value. The return type of the method is the type
// of the defaultValue argument. Currently supported types:
//
//   - string
//   - int
//   - bool
//   - float64
//   - time.Duration
//   - time.Time (must provide format as third argument)
//
// The default is returned if the variable is unset or empty, or if T is not one
// of the supported types. A value that doesn't parse fails the spec, rather
// than silently falling back to the default and hiding a typo.
func GetEnvWithDefault[T any](name string, defaultValue T, options ...any) (value T) {
	GinkgoHelper()

	val := os.Getenv(name)
	if val == "" {
		return defaultValue
	}

	var result any
	var zero T

	switch any(zero).(type) {
	case string:
		result = val
	case int:
		v, err := strconv.Atoi(val)
		ExpectNoError(err, "parse %s=%q as an integer", name, val)
		result = v
	case bool:
		v, err := strconv.ParseBool(val)
		ExpectNoError(err, "parse %s=%q as a boolean", name, val)
		result = v
	case float64:
		v, err := strconv.ParseFloat(val, 64)
		ExpectNoError(err, "parse %s=%q as a float", name, val)
		result = v
	case time.Duration:
		v, err := time.ParseDuration(val)
		ExpectNoError(err, "parse %s=%q as a duration", name, val)
		result = v
	case time.Time:
		if len(options) == 0 {
			return defaultValue
		}
		v, err := tryParseTime(val, options)
		ExpectNoError(err, "parse %s=%q as a time", name, val)
		result = v
	default:
		return defaultValue
	}

	return result.(T)
}

func tryParseTime(val string, formats []any) (time.Time, error) {
	for _, format := range formats {
		switch format := format.(type) {
		case string:
			if res, err := time.Parse(format, val); err == nil {
				return res, nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("invalid time format")
}
