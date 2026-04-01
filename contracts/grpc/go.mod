module github.com/skosovsky/toolsy/contracts/grpc

go 1.26.1

replace github.com/skosovsky/toolsy => ../..

require (
	github.com/skosovsky/toolsy v0.0.0
	google.golang.org/grpc v1.79.2
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	go.opentelemetry.io/otel v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.42.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
)
