// Package envfile supports the Docker/K8s _FILE suffix convention for secrets.
//
// When FOO_FILE=/path/to/secret is set, the value is read from the file
// instead of the FOO env var. This keeps secrets out of /proc/pid/environ.
package envfile

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ReadEnvOrFile returns the value for the given env var key, supporting the
// _FILE suffix convention:
//
//  1. If key+"_FILE" is set, read the file and return its trimmed contents.
//  2. Otherwise, fall back to the plain key env var.
//  3. If both are set, _FILE wins and a warning is logged.
//
// Returns ("", false) if neither is set.
func ReadEnvOrFile(key string, logger *slog.Logger) (string, bool, error) {
	if logger == nil {
		logger = slog.Default()
	}

	fileKey := key + "_FILE"
	filePath, fileSet := os.LookupEnv(fileKey)
	plainVal, plainSet := os.LookupEnv(key)

	if fileSet && filePath != "" {
		if plainSet && plainVal != "" {
			logger.Warn("both env var and _FILE variant set; using _FILE",
				"key", key, "file_key", fileKey)
		}
		data, err := os.ReadFile(filePath) //nolint:gosec // G304: reading secret from user-specified path is the core purpose of this function
		if err != nil {
			return "", false, fmt.Errorf("reading %s=%q: %w", fileKey, filePath, err)
		}
		return strings.TrimSpace(string(data)), true, nil
	}

	if plainSet && plainVal != "" {
		return plainVal, true, nil
	}

	return "", false, nil
}
