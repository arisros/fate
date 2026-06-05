// Command fate inspects statecharts from the command line.
//
// It works on JSON files, so it depends on no particular set of machines:
//
//   - a machine descriptor — the JSON of [fate.MachineDescriptor], as produced
//     by Machine.Describe and marshaled, or served at a studio's
//     /m/{name}/describe endpoint;
//   - a persisted snapshot — the JSON written by Actor.Persist.
//
// Usage:
//
//	fate render   <descriptor.json|->        ASCII state diagram
//	fate mermaid  <descriptor.json|->        Mermaid stateDiagram-v2 source
//	fate graph    <descriptor.json|->        resolved node/edge graph (JSON)
//	fate snap     <snapshot.json|->          inspect a persisted snapshot
//	fate diff     <left.json> <right.json>   structural snapshot diff
//
// A path of "-" reads from standard input.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/arisros/fate"
)

const usage = `fate — inspect statecharts from JSON descriptors and snapshots

Commands:
  fate render   <descriptor.json|->        ASCII state diagram
  fate mermaid  <descriptor.json|->        Mermaid stateDiagram-v2 source
  fate graph    <descriptor.json|->        resolved node/edge graph (JSON)
  fate snap     <snapshot.json|->          inspect a persisted snapshot
  fate diff     <left.json> <right.json>   structural snapshot diff

A path of "-" reads from standard input.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "render":
		runRender(args)
	case "mermaid":
		runMermaid(args)
	case "graph":
		runGraph(args)
	case "snap":
		runSnap(args)
	case "diff":
		runDiff(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "fate: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func runRender(args []string) {
	d := mustDescriptor(arg(args, 0, "render"))
	fmt.Println(fate.RenderASCII(d, fate.RenderOptions{}))
}

func runMermaid(args []string) {
	d := mustDescriptor(arg(args, 0, "mermaid"))
	fmt.Println(fate.RenderMermaid(d, fate.MermaidOptions{}))
}

func runGraph(args []string) {
	d := mustDescriptor(arg(args, 0, "graph"))
	b, err := json.MarshalIndent(fate.RenderGraphJSON(d), "", "  ")
	if err != nil {
		die("graph: marshal: %v", err)
	}
	fmt.Println(string(b))
}

// snapshotShape mirrors fate.Snapshot with the context left as raw bytes, so
// any context type can be inspected without compiling its Go definition.
type snapshotShape struct {
	Version int              `json:"version"`
	Value   fate.StateValue  `json:"value"`
	Context json.RawMessage  `json:"context"`
	Status  fate.ActorStatus `json:"status"`
}

func runSnap(args []string) {
	path := arg(args, 0, "snap")
	var s snapshotShape
	if err := json.Unmarshal(readAll(path), &s); err != nil {
		die("snap: %v", err)
	}
	fmt.Printf("version: %d\n", s.Version)
	fmt.Printf("status:  %s\n", s.Status)
	fmt.Printf("state:   %s\n", s.Value.Path())
	if len(s.Context) > 0 && string(s.Context) != "null" {
		var pretty any
		_ = json.Unmarshal(s.Context, &pretty)
		out, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Printf("context:\n%s\n", string(out))
	}
}

func runDiff(args []string) {
	if len(args) < 2 {
		die("diff: expected <left.json> <right.json>")
	}
	left := readSnapshot(args[0])
	right := readSnapshot(args[1])
	d := fate.DiffSnapshots[json.RawMessage](left, right)
	if d.Empty() {
		fmt.Println("(no differences)")
		return
	}
	fmt.Println("differences (left → right):")
	for _, line := range d.Strings() {
		fmt.Printf("  %s\n", line)
	}
}

func readSnapshot(path string) fate.Snapshot[json.RawMessage] {
	var s snapshotShape
	if err := json.Unmarshal(readAll(path), &s); err != nil {
		die("read %s: %v", path, err)
	}
	return fate.Snapshot[json.RawMessage]{
		Version: s.Version,
		Value:   s.Value,
		Context: s.Context,
		Status:  s.Status,
	}
}

func mustDescriptor(path string) fate.MachineDescriptor {
	d, err := fate.LoadDescriptor(readAll(path))
	if err != nil {
		die("%s: %v", path, err)
	}
	return d
}

func arg(args []string, i int, cmd string) string {
	if len(args) <= i {
		die("%s: expected a file path (or - for stdin)", cmd)
	}
	return args[i]
}

func readAll(path string) []byte {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("read stdin: %v", err)
		}
		return b
	}
	b, err := os.ReadFile(path)
	if err != nil {
		die("read %s: %v", path, err)
	}
	return b
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "fate: "+format+"\n", a...)
	os.Exit(1)
}
