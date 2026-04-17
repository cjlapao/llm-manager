// Package version provides version information for the application.
// Version details can be injected at build time using ldflags:
//
//	go build -ldflags "-X github.com/user/llm-manager/internal/version.version=v1.0.0 \
//	                   -X github.com/user/llm-manager/internal/version.commit=abc123 \
//	                   -X github.com/user/llm-manager/internal/version.date=2024-01-01T00:00:00Z"
//
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// version is the application version, set at build time via ldflags.
var version = "dev"

// commit is set at build time via ldflags.
var commit = "none"

// date is set at build time via ldflags.
var date = "unknown"

// builtBy is set at build time via ldflags.
var builtBy = ""

// Version returns the full version string.
func Version() string {
	return version
}

// Commit returns the git commit hash.
func Commit() string {
	return commit
}

// Date returns the build date in ISO 8601 format.
func Date() string {
	return date
}

// BuiltBy returns the entity that built the binary.
func BuiltBy() string {
	return builtBy
}

// Info returns a formatted multi-line version string including Go runtime info.
func Info() string {
	var b strings.Builder
	fmt.Fprintf(&b, "llm-manager version %s", Version())
	if commit != "none" {
		fmt.Fprintf(&b, " (commit: %s)", shortCommit())
	}
	if date != "unknown" {
		fmt.Fprintf(&b, " built at %s", date)
	}
	if builtBy != "" {
		fmt.Fprintf(&b, " by %s", builtBy)
	}
	fmt.Fprintf(&b, "\ngo version: %s\n", runtime.Version())
	fmt.Fprintf(&b, "architecture: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return b.String()
}

// ShortVersion returns a concise version string suitable for single-line output.
func ShortVersion() string {
	var b strings.Builder
	fmt.Fprintf(&b, "v%s", Version())
	if commit != "none" {
		fmt.Fprintf(&b, "+%s", shortCommit())
	}
	return b.String()
}

// shortCommit returns the short form of the commit hash (first 7 characters).
func shortCommit() string {
	if len(commit) <= 7 {
		return commit
	}
	return commit[:7]
}

// IsDev returns true if the binary was built in development mode (version is "dev").
func IsDev() bool {
	return version == "dev"
}
