module github.com/skosovsky/toolsy/contracts/openapi

go 1.27.1

require (
	github.com/getkin/kin-openapi v0.149.0
	github.com/skosovsky/toolsy v0.0.0
	github.com/skosovsky/toolsy/toolkits/httptool v0.0.0
)

require (
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace (
	github.com/skosovsky/toolsy => ../..
	github.com/skosovsky/toolsy/toolkits/httptool => ../../toolkits/httptool
)
