// scs — statechart-studio CLI. Stdlib-only first cut.
//
// This is the non-interactive subset of the P7 TUI Studio. It exercises
// the descriptor + ASCII renderer + diff pieces of framework/statechart
// without pulling the charmbracelet dep tree. The interactive views
// (sim, timeline, diverge) land once Bubble Tea is added; the data-layer
// commands (view, snap, diff) below produce the same output regardless
// of whether the interactive shell wraps them.
//
// Usage:
//
//	scs list
//	scs view <machine>                     # ASCII state diagram + transition sidebar (root)
//	scs view <machine> <state-path>        # ASCII diagram + transitions out of <state-path>
//	scs describe <machine>                 # JSON descriptor (pretty-printed)
//	scs snap <file.json>                   # Inspect a persisted snapshot
//	scs diff <left.json> <right.json>      # Snapshot diff
//
// Machine names are resolved against the built-in registry below.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	sc "github.com/arisros/fate"
)

const usage = `scs — statechart-studio (data-layer commands)

Commands:
  scs list                            List built-in demo machines
  scs view <machine> [state-path]     Render ASCII state diagram + transition sidebar
  scs describe <machine>              Print JSON descriptor (pretty)
  scs snap <file.json>                Inspect a persisted snapshot
  scs diff <left.json> <right.json>   Structural diff between two snapshots
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "list":
		runList()
	case "view":
		runView(args)
	case "describe":
		runDescribe(args)
	case "snap":
		runSnap(args)
	case "diff":
		runDiff(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "scs: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// runList prints the names + one-line descriptions of registry entries.
func runList() {
	fmt.Println("Built-in demo machines:")
	for _, e := range registry {
		fmt.Printf("  %-18s  %s\n", e.name, e.summary)
	}
	fmt.Println()
	fmt.Println("Hint: run `scs view <name>` to see one rendered.")
}

func runView(args []string) {
	if len(args) < 1 {
		die("view: expected <machine> [state-path]")
	}
	e := mustLookup(args[0])
	d := e.build()
	fmt.Println(sc.RenderASCII(d, sc.RenderOptions{}))
	if len(args) > 1 {
		fmt.Println(sc.RenderTransitions(d, args[1]))
	}
}

func runDescribe(args []string) {
	if len(args) < 1 {
		die("describe: expected <machine>")
	}
	e := mustLookup(args[0])
	d := e.build()
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		die(fmt.Sprintf("describe: marshal: %v", err))
	}
	fmt.Println(string(b))
}

// snapshotShape is the on-disk shape this command accepts. Mirrors
// statechart.Snapshot[json.RawMessage] — context stays as raw bytes so
// the studio can inspect any Ctx without compiling its Go type.
type snapshotShape struct {
	Version int             `json:"version"`
	Value   sc.StateValue   `json:"value"`
	Context json.RawMessage `json:"context"`
	Status  sc.ActorStatus  `json:"status"`
}

func runSnap(args []string) {
	if len(args) < 1 {
		die("snap: expected <file.json>")
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		die(fmt.Sprintf("snap: read: %v", err))
	}
	var s snapshotShape
	if err := json.Unmarshal(b, &s); err != nil {
		die(fmt.Sprintf("snap: unmarshal: %v", err))
	}
	fmt.Printf("Snapshot from %s\n", args[0])
	fmt.Printf("  version: %d\n", s.Version)
	fmt.Printf("  status:  %s\n", s.Status)
	fmt.Printf("  path:    %s\n", s.Value.Path())
	if len(s.Context) > 0 && string(s.Context) != "null" {
		var pretty interface{}
		_ = json.Unmarshal(s.Context, &pretty)
		out, _ := json.MarshalIndent(pretty, "  ", "  ")
		fmt.Printf("  context:\n  %s\n", string(out))
	}
}

func runDiff(args []string) {
	if len(args) < 2 {
		die("diff: expected <left.json> <right.json>")
	}
	left := readSnapshot(args[0])
	right := readSnapshot(args[1])
	d := sc.DiffSnapshots[json.RawMessage](left, right)
	if d.Empty() {
		fmt.Println("(no differences)")
		return
	}
	fmt.Printf("Differences (left → right):\n")
	for _, line := range d.Strings() {
		fmt.Printf("  %s\n", line)
	}
}

func readSnapshot(path string) sc.Snapshot[json.RawMessage] {
	b, err := os.ReadFile(path)
	if err != nil {
		die(fmt.Sprintf("read %s: %v", path, err))
	}
	var s snapshotShape
	if err := json.Unmarshal(b, &s); err != nil {
		die(fmt.Sprintf("unmarshal %s: %v", path, err))
	}
	return sc.Snapshot[json.RawMessage]{
		Version: s.Version,
		Value:   s.Value,
		Context: s.Context,
		Status:  s.Status,
	}
}

func mustLookup(name string) registryEntry {
	for _, e := range registry {
		if e.name == name {
			return e
		}
	}
	available := ""
	for i, e := range registry {
		if i > 0 {
			available += ", "
		}
		available += e.name
	}
	die(fmt.Sprintf("unknown machine %q (available: %s)", name, available))
	return registryEntry{} // unreachable
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "scs: "+msg)
	os.Exit(1)
}

// flag is imported to keep open the obvious extension point: subcommand-
// local flags (e.g. `scs view <machine> --highlight pin_challenge`).
// Routed manually for now since the command surface is small.
var _ = flag.NewFlagSet
