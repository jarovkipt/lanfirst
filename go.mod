module github.com/jarovkipt/lanfirst

go 1.24.0

// Dependencies are resolved by `go mod tidy` (run automatically by install.sh):
//   github.com/caseymrm/menuet   — macOS menu-bar UI (NSStatusItem)
//   github.com/miekg/dns         — DNS server/client
//   github.com/fsnotify/fsnotify — config file watching
//   gopkg.in/yaml.v3             — config parsing
//   golang.org/x/mod             — semver comparison for the in-app updater

require (
	github.com/caseymrm/menuet v1.2.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/miekg/dns v1.1.72
	golang.org/x/mod v0.31.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/caseymrm/askm v1.0.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
)
