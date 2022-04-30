package main

import (
	"fmt"
	"runtime/debug"

	"go.bbkane.com/warg/command"
)

// This will be overriden by goreleaser
var version = "unkown version: error reading goreleaser info"

func getVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown version: error reading build info"
	}
	// go install will embed this
	if info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func printVersion(_ command.Context) error {
	fmt.Println(getVersion())
	return nil
}
