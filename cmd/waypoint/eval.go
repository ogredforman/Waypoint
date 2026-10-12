package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/ogredforman/waypoint/internal/config"
	"github.com/ogredforman/waypoint/internal/eval"
	"github.com/ogredforman/waypoint/internal/extract/provider"
)

// runEval is "waypoint eval": it runs the extraction fixtures against a
// provider and prints a report (SPEC §12). The provider, model, and endpoint
// come from the config unless flags name them; the API key and timeout only
// ever come from the config file or WAYPOINT_* env vars. A -provider other
// than the configured one doesn't inherit the configured model and endpoint,
// which belong to that other provider.
//
// It returns 0 when the eval ran, whatever the score.
func runEval(ctx context.Context, args []string, env config.LookupEnv, stdout, stderr io.Writer) int {
	fset := flag.NewFlagSet("waypoint eval", flag.ContinueOnError)
	fset.SetOutput(stderr)
	flagConfig := fset.String("config", "", "path to config.toml (env WAYPOINT_CONFIG)")
	flagProvider := fset.String("provider", "", "extractor to evaluate: mock, anthropic, or openai-compatible (default: the configured one)")
	flagModel := fset.String("model", "", "model name (default: the configured one, or the provider's)")
	flagEndpoint := fset.String("endpoint", "", "API endpoint (default: the configured one, or the provider's)")
	flagDir := fset.String("dir", "testdata/entries", "fixture directory")
	flagRun := fset.String("run", "", "only run fixtures whose name contains this")
	flagVerbose := fset.Bool("v", false, "show missing and extra ops and links for each fixture")
	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fset.NArg() > 0 {
		fmt.Fprintf(stderr, "waypoint eval: unexpected arguments: %v\n", fset.Args())
		return 2
	}

	var cfgArgs []string
	if *flagConfig != "" {
		cfgArgs = []string{"-config", *flagConfig}
	}
	cfg, err := config.Load(cfgArgs, env, runtime.GOOS, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "waypoint eval:", err)
		return 2
	}
	opts := provider.Options{Provider: cfg.Provider, Model: cfg.Model, Endpoint: cfg.Endpoint, APIKey: cfg.APIKey, Timeout: cfg.Timeout}
	if *flagProvider != "" && *flagProvider != cfg.Provider {
		opts.Provider, opts.Model, opts.Endpoint = *flagProvider, "", ""
	}
	if *flagModel != "" {
		opts.Model = *flagModel
	}
	if *flagEndpoint != "" {
		opts.Endpoint = *flagEndpoint
	}
	x, err := provider.New(opts)
	if err != nil {
		fmt.Fprintln(stderr, "waypoint eval:", err)
		return 2
	}

	all, err := eval.LoadFixtures(*flagDir)
	if err != nil {
		fmt.Fprintln(stderr, "waypoint eval: fixtures:", err)
		return 1
	}
	var fixtures []eval.Fixture
	for _, f := range all {
		if strings.Contains(f.Name, *flagRun) {
			fixtures = append(fixtures, f)
		}
	}
	if len(fixtures) == 0 {
		fmt.Fprintln(stderr, "waypoint eval: no fixtures to run")
		return 1
	}

	header := "waypoint eval: " + x.Name()
	if m, ok := x.(interface{ Model() string }); ok {
		header += " " + m.Model()
	}
	if p, ok := x.(interface{ PromptVersion() string }); ok {
		header += ", prompt " + p.PromptVersion()
	}
	header += fmt.Sprintf(", %d fixtures", len(fixtures))

	n := 0
	outs := eval.Run(ctx, x, fixtures, func(o eval.Outcome) {
		n++
		fmt.Fprintf(stderr, "[%d/%d] %s %s\n", n, len(fixtures), o.Fixture, o.Status)
	})
	if err := eval.WriteReport(stdout, header, outs, *flagVerbose); err != nil {
		fmt.Fprintln(stderr, "waypoint eval:", err)
		return 1
	}
	if len(outs) < len(fixtures) {
		fmt.Fprintf(stderr, "waypoint eval: stopped after %d of %d fixtures\n", len(outs), len(fixtures))
		return 1
	}
	return 0
}
