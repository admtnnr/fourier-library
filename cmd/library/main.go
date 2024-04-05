// library is a simple library management system that reads a list of commands
// from a file and executes them against the library system.
//
// library [flags] <commands-file>
//
// Flags:
//
//	--db string         path to DB file (default "state.db")
//	--help              display help and exits
//
// The <commands-file> can be a file or stdin. If the file is "-", then stdin
// is used.
//
// The commands file is a newline-delimited JSON file with one command per
// line. Each command is JSON object with the following structure:
//
//	{
//		"name": "command-name",
//		"arguments": {
//			"arg1": "value1",
//			"arg2": "value2",
//			...
//		}
//	}
//
// The following commands are supported:
// - ADD_BOOK
// - CREATE_ACCOUNT
// - CHECKOUT_BOOK
// - RETURN_BOOK
// - ADD_COPIES
// - REMOVE_COPIES
// - PRINT_CATALOG
// - PRINT_ACCOUNTS
//
// Commands are executed in the order they appear in the file. If any command
// fails, the program will exit with a non-zero exit code. Any changes made to
// the library system prior to the failure will *NOT* be persisted back to the
// DB.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/admtnnr/library"
)

var (
	dbPath = flag.String("db", "state.db", "path to DB file")

	usage = `library is a simple library management system that reads a list of commands
from a file and executes them against the library system.

library [flags] <commands-file>

The <commands-file> can be a file or stdin. If the file is "-", then stdin
is used.

Flags:

     --db string         path to DB file (default "state.db")
     --help              display help and exits
`
)

func init() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
}

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	l := library.New()

	db, err := os.OpenFile(*dbPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to open library DB, %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := l.Import(db, library.ImportOptions{}); err != nil {
		fmt.Fprintf(os.Stdout, "failed to load library DB from %s, %v\n", *dbPath, err)
		os.Exit(1)
	}

	commandsPath := flag.Arg(0)
	var commands io.ReadCloser

	if commandsPath == "-" {
		commands = os.Stdin
	} else {
		var err error

		if commands, err = os.Open(commandsPath); err != nil {
			fmt.Fprintf(os.Stdout, "failed to open commands file, %v\n", err)
			os.Exit(1)
		}
		defer commands.Close()
	}

	if err := l.Import(commands, library.ImportOptions{LogOutput: true}); err != nil {
		fmt.Fprintf(os.Stdout, "failed to execute commands from %s, %v\n", commandsPath, err)
		os.Exit(1)
	}

	// Create a temporary file to export the library state to before we
	// replace the existing library state file.
	//
	// We do this in an attempt to ensure that we do not lose or corrupt
	// the existing library state if the export fails during some
	// combination of truncating and writing directly into the existing
	// state file.
	export, err := os.CreateTemp("", "state.db")
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to create temporary export file, %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(export.Name())

	if err := l.Export(export); err != nil {
		fmt.Fprintf(os.Stdout, "failed to save library state to DB, %v\n", err)
		os.Exit(1)
	}

	// Force the export file to be written to disk before we replace the existing
	// library state file.
	export.Sync()

	// Rename is atomic on Linux systems, so we should not lose the
	// existing library state should it fail.
	if err := os.Rename(export.Name(), *dbPath); err != nil {
		fmt.Fprintf(os.Stdout, "failed to replace library DB file, %v\n", err)
		os.Exit(1)
	}
}
