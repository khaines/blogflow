// Package envfile supports the Docker/K8s _FILE suffix convention for secrets.
//
// When FOO_FILE=/path/to/secret is set, the value is read from the file
// instead of the FOO env var. This keeps secrets out of /proc/pid/environ.
package envfile

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// maxSecretSize caps secret file reads at 1 MiB to prevent OOM from
// misconfigured paths (e.g. pointing at a device or huge file).
const maxSecretSize = 1 << 20

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
		val, err := readSecretFile(filePath, logger)
		if err != nil {
			return "", false, fmt.Errorf("reading %s=%q: %w", fileKey, filePath, err)
		}
		return strings.TrimSpace(val), true, nil
	}

	if plainSet && plainVal != "" {
		return plainVal, true, nil
	}

	return "", false, nil
}

// readSecretFile opens path, validates it is a regular file within the size
// limit, warns on overly permissive permissions, and returns the contents.
// It uses Fstat on the opened fd to avoid TOCTOU races.
func readSecretFile(path string, logger *slog.Logger) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path is from operator-controlled env var; this is the core purpose
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return "", err
	}

	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file (mode: %s)", fi.Mode().Type())
	}

	if fi.Mode().Perm()&0o004 != 0 {
		logger.Warn("secret file is world-readable; consider chmod 0600",
			"path", path, "mode", fmt.Sprintf("%04o", fi.Mode().Perm()))
	}

	if fi.Size() > maxSecretSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", fi.Size(), maxSecretSize)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxSecretSize+1))
	if err != nil {
		return "", err
	}

	if int64(len(data)) > maxSecretSize {
		return "", fmt.Errorf("file too large (read %d bytes, max %d)", len(data), maxSecretSize)
	}

	return string(data), nil
}
