package main

import (
	"flag"
	"log"

	"github.com/crossplane/function-sdk-go"
)

func main() {
	insecure := flag.Bool("insecure", false, "Run without mTLS. Use only for local crossplane render development.")
	flag.Parse()

	opts := []function.ServeOption{}
	if *insecure {
		opts = append(opts, function.Insecure(true))
	}

	if err := function.Serve(&Function{}, opts...); err != nil {
		log.Fatalf("cannot serve function: %v", err)
	}
}
