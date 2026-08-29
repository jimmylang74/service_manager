//go:build windows

package main

import "flag"

// scmOpt holds the -scm flag. It is defined only on Windows, where a console
// run needs an explicit way to request system service registration.
var scmOpt *bool

func registerSCMFlag() {
	scmOpt = flag.Bool("scm", false, "run as a system service (register and start it when launched under SCM)")
}