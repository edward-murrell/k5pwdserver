// Package version holds the package name and version shown in the page footer.
package version

// Package is the name of the distributed package.
const Package = "k8init"

// Version is set at build time with:
//
//	go build -ldflags "-X github.com/edward-murrell/k8init/internal/version.Version=1.2.3"
var Version = "dev"
