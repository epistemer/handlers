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
	github.com/getkin/kin-openapi v0.153.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/itchyny/gojq v0.12.20 // indirect
	github.com/itchyny/timefmt-go v0.1.9 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
)
