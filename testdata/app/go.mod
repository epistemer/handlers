module app

go 1.26.0

replace github.com/epistemer/handlers => ../..

replace github.com/epistemer/handlers/authguard => ../../authguard

replace github.com/epistemer/handlers/cmd => ../../cmd

tool (
	github.com/epistemer/genx/cmd/genx
	github.com/epistemer/handlers/cmd/handlers
)

require (
	github.com/epistemer/auth v0.3.0
	github.com/epistemer/genx v0.2.0
	github.com/epistemer/handlers v0.0.0
	github.com/epistemer/handlers/authguard v0.0.0
	github.com/epistemer/i18n v0.3.1
	go.etcd.io/bbolt v1.5.0
)

require (
	github.com/epistemer/cfgx v0.2.0 // indirect
	github.com/epistemer/errors v0.1.0 // indirect
	github.com/epistemer/handlers/cmd v0.0.0 // indirect
	github.com/itchyny/gojq v0.12.20 // indirect
	github.com/itchyny/timefmt-go v0.1.9 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
