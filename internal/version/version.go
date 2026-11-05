// Package version holds the package name and version shown in the page footer.
package version

// Package is the name of the distributed package.
const Package = "k5pwdserver"

// Version is set at build time with:
//
//	go build -ldflags "-X github.com/edward-murrell/k5pwdserver/internal/version.Version=1.2.3"
var Version = "dev"
