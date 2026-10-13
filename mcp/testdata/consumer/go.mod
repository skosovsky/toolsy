module github.com/skosovsky/toolsy/mcp/testdata/consumer

go 1.27.1

require (
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/skosovsky/toolsy v0.0.0
	github.com/skosovsky/toolsy/mcp v0.0.0
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

// Local source replacements test the unpublished candidate without go.work.
// The SDK replacement matches the MCP module dependency.
replace github.com/skosovsky/toolsy => ../../../

replace github.com/skosovsky/toolsy/mcp => ../../

replace github.com/modelcontextprotocol/go-sdk => github.com/skosovsky/go-sdk v1.8.1-0.20261013081052-4a7b89fe086f
