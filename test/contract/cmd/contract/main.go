// Command contract records the v1 contract from a real Alertmanager v0.26.0
// and verifies any target URL against it. See test/contract/README.md.
//
//	contract record  -target URL -out DIR
//	contract verify  -target URL -fixtures DIR [-pending FILE] [-v]
//	contract compare -a DIR -b DIR
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"alertmanager-proxy/test/contract/harness"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	ok := true
	switch os.Args[1] {
	case "record":
		err = record(os.Args[2:])
	case "verify":
		ok, err = verify(os.Args[2:])
	case "compare":
		ok, err = compare(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "contract:", err)
		os.Exit(2)
	}
	if !ok {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: contract record|verify|compare [flags]")
	os.Exit(2)
}

type common struct {
	target, scenario string
	readyTimeout     time.Duration
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.target, "target", "", "base URL of the Alertmanager or proxy under test")
	fs.StringVar(&c.scenario, "scenario", "test/contract/scenario.yaml", "scenario file")
	fs.DurationVar(&c.readyTimeout, "ready-timeout", 60*time.Second, "how long to wait for GET /-/ready")
}

func (c *common) run(strict bool) (harness.Run, error) {
	if c.target == "" {
		return harness.Run{}, fmt.Errorf("-target is required")
	}
	sc, err := harness.LoadScenario(c.scenario)
	if err != nil {
		return harness.Run{}, err
	}
	r := &harness.Runner{
		Target: c.target,
		Client: &http.Client{Timeout: 10 * time.Second},
		Strict: strict,
		Logf:   func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
	}
	ctx := context.Background()
	if err := r.WaitReady(ctx, c.readyTimeout); err != nil {
		return harness.Run{}, err
	}
	return r.Execute(ctx, sc)
}

func record(args []string) error {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	var c common
	c.register(fs)
	out := fs.String("out", "", "fixture directory to (re)write")
	_ = fs.Parse(args)
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	run, err := c.run(true)
	if err != nil {
		return err
	}
	if err := harness.Save(run, *out); err != nil {
		return err
	}
	fmt.Printf("recorded %d exchanges from Alertmanager %s into %s\n", len(run.Exchanges), run.Meta.AlertmanagerVersion, *out)
	return nil
}

func verify(args []string) (bool, error) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	var c common
	c.register(fs)
	fixtures := fs.String("fixtures", "test/contract/fixtures/v0.26.0", "recorded fixture directory")
	pendingPath := fs.String("pending", "test/contract/pending.txt", "endpoints expected to fail (empty: none)")
	strict := fs.Bool("strict", false, "release mode: fail if any endpoint is pending")
	verbose := fs.Bool("v", false, "print diffs for pending endpoints too")
	_ = fs.Parse(args)

	want, err := harness.Load(*fixtures)
	if err != nil {
		return false, err
	}
	if sc, err := harness.LoadScenario(c.scenario); err == nil && sc.SHA256 != want.Meta.ScenarioSHA256 {
		return false, fmt.Errorf("scenario changed since fixtures were recorded: re-record (make contract-record)")
	}
	pending, err := harness.LoadPending(*pendingPath)
	if err != nil {
		return false, err
	}
	if *strict {
		if err := harness.CheckStrictPending(pending); err != nil {
			return false, err
		}
	}
	got, err := c.run(false)
	if err != nil {
		return false, err
	}
	fmt.Printf("fixtures: Alertmanager %s · target: %s (upstream reports %s)\n\n",
		want.Meta.AlertmanagerVersion, got.Meta.Target, got.Meta.AlertmanagerVersion)
	return harness.Report(os.Stdout, harness.Compare(want, got), pending, *verbose), nil
}

func compare(args []string) (bool, error) {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	a := fs.String("a", "", "first fixture directory")
	b := fs.String("b", "", "second fixture directory")
	_ = fs.Parse(args)
	ra, err := harness.Load(*a)
	if err != nil {
		return false, err
	}
	rb, err := harness.Load(*b)
	if err != nil {
		return false, err
	}
	return harness.Report(os.Stdout, harness.Compare(ra, rb), nil, true), nil
}
