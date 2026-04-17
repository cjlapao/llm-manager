// Package version provides a public API for accessing version information.
//
// This package re-exports version information from the internal version package
// to allow other packages to query the application version without importing
// internal packages.
package version

import (
	"github.com/user/llm-manager/internal/version"
)

// Version returns the application version string.
func Version() string {
	return version.Version()
}

// Commit returns the git commit hash.
func Commit() string {
	return version.Commit()
}

// Date returns the build date.
func Date() string {
	return version.Date()
}

// Info returns the full version information string.
func Info() string {
	return version.Info()
}

// ShortVersion returns the short version string.
func ShortVersion() string {
	return version.ShortVersion()
}

// IsDev returns true if this is a development build.
func IsDev() bool {
	return version.IsDev()
}
