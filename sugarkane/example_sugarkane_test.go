package sugarkane_test

import (
	"os"

	"github.com/bbkane/grabbit/sugarkane"
	"go.uber.org/zap"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// https://blog.golang.org/examples
func Example() {
	// intialize a more useful lumberjack.Logger with:
	//   https://github.com/natefinch/lumberjack
	var lumberjackLogger *lumberjack.Logger = nil
	sk := sugarkane.NewSugarKane(lumberjackLogger, os.Stderr, os.Stdout, zap.DebugLevel, "v1.0.0")
	defer sk.Sync()
	sk.LogOnPanic()
	sk.Infow(
		"Now we're logging :)",
		"key", "value",
	)
}
