module github.com/skosovsky/toolsy/adapters/sandbox/starlark

go 1.27.1

require (
	github.com/skosovsky/toolsy v0.0.0
	github.com/stretchr/testify v1.12.1
	go.starlark.net v0.0.0-20260930220527-d7438c5a85ac
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/skosovsky/toolsy => ../../..
