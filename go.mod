module github.com/furbyhaxx/pig-workbench

go 1.26.0

require (
	github.com/BurntSushi/toml v1.6.0
	// PiG resolves this requirement to the version-matched staged SDK at build time.
	github.com/MichaelKinsy/PiG/extensions/sdk v0.0.0
	github.com/creachadair/tomledit v0.0.29
	github.com/gofrs/flock v0.13.1
)

require (
	github.com/nikolalohinski/gonja/v2 v2.9.1
	github.com/odvcencio/gotreesitter v0.56.0
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.23.0
)
