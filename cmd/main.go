package main

import (
	"github.com/couchbaselabs/eviction-reschedule-hook/pkg/reschedule"
)

func main() {
	reschedule.Serve()
}
