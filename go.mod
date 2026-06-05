// Module fate is a Harel statechart engine for Go.
//
// The root module is intentionally dependency-free: it imports only the
// standard library. The Temporal integration lives in a separate module
// (./temporal, github.com/arisros/fate/temporal) so that adopters who only
// want the engine never pull in the Temporal SDK or its transitive graph.
//
// Test-only dependencies (e.g. the property-based testing library) are listed
// below and are excluded from any non-test build by the Go toolchain, so the
// shipped library remains zero-dependency.
module github.com/arisros/fate

go 1.24
