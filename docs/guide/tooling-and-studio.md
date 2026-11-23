# Tooling and the studio

[fate-studio](https://github.com/arisros/fate-studio) draws a machine and lets you
step through it: send events, fire timers, resolve invocations, and jump from a
state to the Go that defines it. It connects to your program at one of three
levels, and you can combine them.

| Level | What your program adds | What you get | Your program imports |
| --- | --- | --- | --- |
| Snapshot | A command that writes `.fate/*.json` | A static chart | `fate` only |
| Proxy | `httphandler` mounted in dev or staging | A live simulator of your real machine | `fate` only |
| Embed | `fate-studio` mounted in your server | The studio inside your program | `fate` and `fate-studio` |

For most projects, use snapshots and the proxy together and run the studio as a
separate binary. Your program keeps the engine's zero dependencies, one studio
serves the machines of every service, and chart changes show up in code review.

A runnable version of the first two levels is in
[`examples/studio-proxy`](https://github.com/arisros/fate/tree/main/examples/studio-proxy).

## Snapshots

`snapshot.Emit` writes the machine's descriptor, the JSON that `Describe`
returns, to `<dir>/<name>.json`:

```go
if err := snapshot.Emit(".fate", "checkout", checkoutMachine()); err != nil {
	log.Fatal(err)
}
```

Put this in a small command (or behind a flag, as the example does) and point
the studio at the directory:

```sh
go install github.com/arisros/fate-studio/cmd/fate-studio@latest
fate-studio --snapshots .fate --watch
# open http://localhost:8090
```

`--watch` reloads the chart each time the file changes. Commit `.fate/` and let
CI fail when it is stale, so every change to a machine is visible in review:

```sh
go run ./cmd/emit
git diff --exit-code .fate
```

A snapshot has no runtime behind it. To simulate it, add the proxy.

## Live simulation through httphandler

`httphandler.New` turns a machine into the simulator API the studio speaks.
Mount it under a prefix of your own:

```go
if os.Getenv("FATE_SIM") == "1" {
	mux.Handle("/fate/checkout/", http.StripPrefix("/fate/checkout",
		httphandler.New(checkoutMachine(), dispatch)))
}
```

Then tell the studio where it is. The variable is `FATE_PROXY_` followed by the
machine name in upper case, with every character other than a letter or digit
replaced by `_` (`order-v2` reads `FATE_PROXY_ORDER_V2`):

```sh
FATE_PROXY_CHECKOUT=http://localhost:8080/fate/checkout \
  fate-studio --snapshots .fate --watch
```

The snapshot named `checkout` becomes live. For several machines, list them in a
config file instead and start the studio with `--config studio.json`:

```json
{
  "snapshots": [".fate"],
  "watch": true,
  "proxyURLs": {
    "checkout": "http://localhost:8080/fate/checkout",
    "order-v2": "http://orders.dev.internal/fate/order-v2"
  }
}
```

### The dispatch function

The simulator sends events by name only, with no payload. `dispatch` turns a name
into one of your events and supplies sample values for its fields:

```go
var events = map[string]func() event{
	"ADD_ITEM": func() event { return addItem{SKU: "sku-demo"} },
	"CHECKOUT": func() event { return checkout{} },
}

func dispatch(name string) (event, error) {
	build, ok := events[name]
	if !ok {
		return nil, httphandler.ErrUnknownEvent{Name: name}
	}
	return build(), nil
}
```

Use the same names your transitions use in `On`. A struct event gets its name
from an `EventName() string` method, or from its type name; see
[defining machines](defining-machines.md).

### What the handler serves

Each browser gets its own actor, kept in memory and keyed by a cookie, and
dropped after 30 minutes without use. It never touches your real actors or your
database.

| Path | Method | Does |
| --- | --- | --- |
| `stream` | GET | Server-Sent Events with the snapshot after every step |
| `send` | POST | Sends the event named by the form field `event` |
| `timer` | POST | Fires the pending timer `id` |
| `invoke` | POST | Resolves the pending invocation `id` (`action=reject` with `error` to fail it, `output` to pass a result) |
| `reset`, `undo` | POST | Starts over, or steps back once |
| `timeline`, `export`, `import` | GET, GET, POST | The step history, and the session as a persisted snapshot |
| `sources` | GET | Where each state, guard and action is written |

Timer and invocation ids are the ones the snapshot lists, such as
`paying#invoke#charge`.

## Embedding the studio

When your program is already an internal tool with its own login, you can mount
the whole studio in it:

```go
srv := studio.NewServer("shop")
srv.SetBasePath("/studio/")
srv.Register(studio.Entry{
	Name:    "checkout",
	Summary: "the checkout flow",
	Build:   checkoutMachine().Describe,
	BuildLive: func() studio.LiveInstance {
		return studio.NewLiveActor(checkoutMachine(), dispatch, checkoutMachine().Describe)
	},
})
mux.Handle("/studio/", http.StripPrefix("/studio", srv.Handler()))
```

This adds `fate-studio`, its web server and its built UI to your module. See the
[fate-studio README](https://github.com/arisros/fate-studio#embed-it-in-your-own-program)
for editor links and loading snapshots alongside live machines.

## Building your own tooling

The studio uses only public engine API, so a dashboard or admin page of your own
can use the same data without the studio:

| API | Gives |
| --- | --- |
| `Machine.Describe` | The chart as a JSON `MachineDescriptor` |
| `Machine.Sources` | The file and line of each state, guard and action |
| `Machine.Lint` | Problems such as transitions that can never fire |
| `Machine.UIState` | The view model of each active state |
| `Actor.NextEvents` | The events the current state handles |
| `Actor.Preview` | The snapshot an event would produce, without sending it |
| `Actor.SubscribeSteps` | A record of every step as it happens |

## Security

Neither `httphandler` nor the studio has authentication. Anyone who can reach
them can drive simulations and read your source locations, and a studio started
with `--editor-cmd` runs a command on its host. Mount them only in development
or staging, behind a flag such as `FATE_SIM`, or behind your own auth. Never
expose them from production.
