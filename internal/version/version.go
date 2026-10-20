package version

import "runtime"

var Version = "dev"
var Commit = "unknown"

func String() string { return Version + " (commit " + Commit + ", " + runtime.Version() + ")" }
