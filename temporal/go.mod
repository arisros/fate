// Module fate/temporal is the Temporal integration for the fate statechart
// engine. It is a separate module from github.com/arisros/fate by design: the
// engine itself stays dependency-free, and only adopters who drive machines
// inside Temporal workflows pull in the Temporal SDK.
//
// During local development this module resolves the engine via the replace
// directive below. Released builds pin a published github.com/arisros/fate tag
// and the replace directive is dropped.
module github.com/arisros/fate/temporal

go 1.25.4

require (
	github.com/arisros/fate v0.4.0
	github.com/stretchr/testify v1.12.1
	go.temporal.io/sdk v1.48.0
)

require (
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.22.0 // indirect
	github.com/nexus-rpc/nexus-proto-annotations v0.1.0 // indirect
	github.com/nexus-rpc/sdk-go v0.7.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.temporal.io/api v1.63.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/arisros/fate => ../
