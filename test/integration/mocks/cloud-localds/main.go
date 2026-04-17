// Mock cloud-localds used by the holos integration test suite.
//
// Real cloud-localds assembles a NoCloud seed ISO from user-data and
// meta-data files. The integration tests do not boot a real VM, so this
// mock simply concatenates the input files into the target path to prove
// the runtime invoked the builder with sensible arguments.
//
// Supported invocation (matches internal/runtime/seed.go):
//
//	cloud-localds [--network-config <path>] <output> <user-data> <meta-data>
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	var (
		networkConfig string
		positional    []string
	)
	for i := 0; i < len(args); i++ {
		if args[i] == "--network-config" && i+1 < len(args) {
			networkConfig = args[i+1]
			i++
			continue
		}
		positional = append(positional, args[i])
	}

	if len(positional) < 3 {
		fmt.Fprintf(os.Stderr, "cloud-localds: expected <output> <user-data> <meta-data>, got %v\n", args)
		os.Exit(1)
	}

	output := positional[0]
	inputs := positional[1:]
	if networkConfig != "" {
		inputs = append(inputs, networkConfig)
	}

	var builder strings.Builder
	for _, in := range inputs {
		data, err := os.ReadFile(in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cloud-localds: read %s: %v\n", in, err)
			os.Exit(1)
		}
		builder.WriteString("# ")
		builder.WriteString(in)
		builder.WriteString("\n")
		builder.Write(data)
		builder.WriteString("\n")
	}

	if err := os.WriteFile(output, []byte(builder.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cloud-localds: write %s: %v\n", output, err)
		os.Exit(1)
	}
}
