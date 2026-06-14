module github.com/skosovsky/toolsy/contracts/graphql

go 1.26.3

require (
	github.com/skosovsky/toolsy v0.0.0
	github.com/skosovsky/toolsy/toolkits/httptool v0.0.0
)

require github.com/google/jsonschema-go v0.4.2 // indirect

replace (
	github.com/skosovsky/toolsy => ../..
	github.com/skosovsky/toolsy/toolkits/httptool => ../../toolkits/httptool
)
