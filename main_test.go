package main

import (
	"testing"
)

func TestApp_Validate(t *testing.T) {
	t.Parallel()
	app := app()

	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}
