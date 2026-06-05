// Module fate/temporal is the Temporal integration for the fate statechart
// engine. It is a separate module from github.com/arisros/fate by design: the
// engine itself stays dependency-free, and only adopters who drive machines
// inside Temporal workflows pull in the Temporal SDK.
//
// During local development this module resolves the engine via the replace
// directive below. Released builds pin a published github.com/arisros/fate tag
// and the replace directive is dropped.
module github.com/arisros/fate/temporal

go 1.24.0

require (
	github.com/arisros/fate v0.0.0
	github.com/stretchr/testify v1.10.0
	go.temporal.io/sdk v1.44.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.22.0 // indirect
	github.com/nexus-rpc/sdk-go v0.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	go.temporal.io/api v1.62.12 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/arisros/fate => ../
