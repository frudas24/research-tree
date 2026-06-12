// Command rt is the CLI for research-tree, a tool for mapping scientific
// research as a directed acyclic graph (DAG) of nodes and edges.
package main

import (
	"fmt"
	"os"

	"github.com/frudas24/research-tree/cmd/rt/cmds"
)

// main is the entry point for the research-tree CLI.
func main() {
	root := cmds.NewRootCmd()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "rt: %v\n", err)
		os.Exit(1)
	}
}
