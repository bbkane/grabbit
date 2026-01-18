package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.bbkane.com/warg"
)

func TestGrabE2E(t *testing.T) {
	t.Parallel()
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
		// 2026-01-17: for some reason "day" is returning an empty list
		"--subreddit-info", "wallpapers,week,1",
		"--destination", dir,
		// hack to not use a config
		"--config", "not-there",
		"--log-filename", filepath.Join(dir, "log.jsonl"),
	}

	parsed, err := app.Parse(
		warg.ParseWithArgs(args),
		warg.ParseWithLookupEnv(warg.LookupMap(nil)),
	)
	require.Nil(t, err)

	err = parsed.Action(parsed.Context)
	require.Nil(t, err)

}
func TestCheckConfigVersionKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		filename        string
		requiredVersion string
		wantErr         bool
	}{
		{
			name:            "devel version always passes",
			filename:        "testdata/TestCheckConfigVersionKey/valid.yaml",
			requiredVersion: "(devel)",
			wantErr:         false,
		},
		{
			name:            "matching major versions",
			filename:        "testdata/TestCheckConfigVersionKey/v1.0.yaml",
			requiredVersion: "1.2.3",
			wantErr:         false,
		},
		{
			name:            "mismatched major versions",
			filename:        "testdata/TestCheckConfigVersionKey/v2.0.yaml",
			requiredVersion: "1.0.0",
			wantErr:         true,
		},
		{
			name:            "file not found",
			filename:        "testdata/TestCheckConfigVersionKey/nonexistent.yaml",
			requiredVersion: "1.0.0",
			wantErr:         true,
		},
		{
			name:            "invalid yaml",
			filename:        "testdata/TestCheckConfigVersionKey/invalid.notyaml",
			requiredVersion: "1.0.0",
			wantErr:         true,
		},
		{
			name:            "just major version",
			filename:        "testdata/TestCheckConfigVersionKey/v1.yaml",
			requiredVersion: "1.0.0",
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkConfigVersionKey(tt.filename, tt.requiredVersion)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
