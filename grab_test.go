package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.bbkane.com/warg"
)

func TestGrabE2E(t *testing.T) {
	// A non-deterministic test,so we do have to inspect the output...
	// But it's the best way to determine if reddit really works too

	run := os.Getenv("GRABBIT_TEST_RUN_E2E") != ""
	if !run {
		t.Skip("Skipping Test. Run with\n  GRABBIT_TEST_RUN_E2E=1 go test ./...")
	}

	dir, err := os.MkdirTemp("", "grabbit-grab-e2e")
	require.Nil(t, err)
	t.Logf("Using tmpdir: %s", dir)

	app := app()
	args := []string{
		"grabbit", "grab",
		"--subreddit-destination", dir,
		"--subreddit-limit", "1",
		"--subreddit-name", "wallpapers",
		"--subreddit-timeframe", "day",
		// hack to not use a config
		"--config", "not-there",
		"--log-filename", filepath.Join(dir, "log.jsonl"),
	}

	parsed, err := app.Parse(args, warg.LookupMap(nil))
	require.Nil(t, err)

	err = parsed.Action(parsed.Context)
	require.Nil(t, err)

}
