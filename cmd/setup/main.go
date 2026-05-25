package main

import (
	"flag"
	"fmt"
	"os"

	"corpdictation/internal/setup"
)

func main() {
	var root string
	flag.StringVar(&root, "root", "staging/windows-localappdata/CorpDictation", "staging root that mirrors %LOCALAPPDATA%\\CorpDictation")
	flag.Parse()

	if err := setup.Run(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
