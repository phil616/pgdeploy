// Package version holds build metadata for the pgdeploy executable.
package version

// Version is the semantic version of the installer.
const Version = "1.0.0"

// Product is the configuration/product format name used across the installer.
const Product = "PGDeploy"

// Build metadata, injected at link time where available.
var (
	GitCommit = "unknown"
	BuildTime = "unknown"
)
