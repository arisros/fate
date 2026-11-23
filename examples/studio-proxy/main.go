package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/engine"
	"github.com/arisros/fate/httphandler"
	"github.com/arisros/fate/snapshot"
)

type cart struct {
	Items int
	SKUs  []string
}

type event interface{ isEvent() }

type addItem struct{ SKU string }
type checkout struct{}
type paymentSucceeded struct{}
type paymentFailed struct{}

func (addItem) isEvent()          {}
func (checkout) isEvent()         {}
func (paymentSucceeded) isEvent() {}
func (paymentFailed) isEvent()    {}

func (addItem) EventName() string          { return "ADD_ITEM" }
func (checkout) EventName() string         { return "CHECKOUT" }
func (paymentSucceeded) EventName() string { return "PAYMENT_SUCCEEDED" }
func (paymentFailed) EventName() string    { return "PAYMENT_FAILED" }

type transitions = map[string][]engine.TransitionConfig[cart, event]

func checkoutMachine() *engine.Machine[cart, event] {
	m, err := engine.CreateMachine(engine.MachineConfig[cart, event]{
		ID:      "checkout",
		Initial: "shopping",
		States: map[string]engine.StateNodeConfig[cart, event]{
			"shopping": {
				On: transitions{
					"ADD_ITEM": {{Actions: []action.Action[cart, event]{
						action.Named("addToCart", action.Assign(func(c cart, e event) cart {
							c.Items++
							c.SKUs = append(append([]string(nil), c.SKUs...), e.(addItem).SKU)
							return c
						})),
					}}},
					"CHECKOUT": {{
						Target: "paying",
						Guard:  func(c cart, _ event) bool { return c.Items > 0 },
					}},
				},
			},
			"paying": {
				Invoke: []effect.Invocation[cart, event]{{
					ID:      "charge",
					Src:     "chargeCard",
					Input:   func(c cart) any { return c.SKUs },
					OnDone:  func(any) event { return paymentSucceeded{} },
					OnError: func(error) event { return paymentFailed{} },
				}},
				After: map[time.Duration][]engine.TransitionConfig[cart, event]{
					30 * time.Second: {{Target: "shopping"}},
				},
				On: transitions{
					"PAYMENT_SUCCEEDED": {{Target: "paid"}},
					"PAYMENT_FAILED":    {{Target: "shopping"}},
				},
			},
			"paid": {Type: engine.NodeFinal},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return m
}

var events = map[string]func() event{
	"ADD_ITEM":          func() event { return addItem{SKU: "sku-demo"} },
	"CHECKOUT":          func() event { return checkout{} },
	"PAYMENT_SUCCEEDED": func() event { return paymentSucceeded{} },
	"PAYMENT_FAILED":    func() event { return paymentFailed{} },
}

func dispatch(name string) (event, error) {
	build, ok := events[name]
	if !ok {
		return nil, httphandler.ErrUnknownEvent{Name: name}
	}
	return build(), nil
}

func main() {
	emit := flag.String("emit", "", "write the machine's descriptor to this directory and exit")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	m := checkoutMachine()

	if *emit != "" {
		if err := snapshot.Emit(*emit, "checkout", m); err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s/checkout.json", *emit)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	if os.Getenv("FATE_SIM") == "1" {
		mux.Handle("/fate/checkout/", http.StripPrefix("/fate/checkout", httphandler.New(m, dispatch)))
		log.Printf("simulator at http://localhost%s/fate/checkout", *addr)
	}

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
