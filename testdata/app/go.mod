module app

go 1.26.0

replace github.com/epistemer/handlers => ../..

replace github.com/epistemer/handlers/authguard => ../../authguard

replace github.com/epistemer/handlers/cmd => ../../cmd

replace github.com/epistemer/handlers/openapi => ../../openapi

tool (
	github.com/epistemer/genx/cmd/genx
	github.com/epistemer/handlers/cmd/handlers
	github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
)

require (
	github.com/epistemer/auth v0.3.0
	github.com/epistemer/genx v0.2.0
	github.com/epistemer/handlers v0.3.0
	github.com/epistemer/handlers/authguard v0.0.0
	github.com/epistemer/handlers/openapi v0.0.0
	github.com/epistemer/i18n v0.3.1
	github.com/getkin/kin-openapi v0.153.0
	github.com/oapi-codegen/runtime v1.9.0
	github.com/shopspring/decimal v1.4.0
	go.etcd.io/bbolt v1.5.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/epistemer/cfgx v0.2.0 // indirect
	github.com/epistemer/errors v0.1.0 // indirect
	github.com/epistemer/handlers/cmd v0.0.0 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/itchyny/gojq v0.12.20 // indirect
	github.com/itchyny/timefmt-go v0.1.9 // indirect
	github.com/oapi-codegen/oapi-codegen/v2 v2.8.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/speakeasy-api/openapi v1.24.0 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
