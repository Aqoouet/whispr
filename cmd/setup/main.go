package main

import (
	"flag"
	"fmt"
	"os"

	"corpdictation/internal/setup"
)

func main() {
	var root string
	flag.StringVar(&root, "root", "staging/windows-localappdata/CorpDictation", "staging root for the local layout mirror; deployed runtime resolution may use CORPDICTATION_ROOT or a machine-wide Windows root")
	flag.Parse()

	if err := setup.Run(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
