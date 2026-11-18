module genxapp

go 1.26.0

replace github.com/epistemer/handlers => ../../..

replace github.com/epistemer/handlers/cmd => ../..

tool (
	github.com/epistemer/genx/cmd/genx
	github.com/epistemer/handlers/cmd/handlers
)

require github.com/epistemer/genx v0.2.0

require (
	github.com/epistemer/handlers v0.0.0 // indirect
	github.com/epistemer/handlers/cmd v0.0.0 // indirect
	github.com/itchyny/gojq v0.12.20 // indirect
	github.com/itchyny/timefmt-go v0.1.9 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
)
