// Command falatracker monitors the Falabella Business Center purchase orders
// and downloads the Excel reports of new ones.
//
// Run `falatracker --help` for the available flags.
package main

import (
	"os"

	"github.com/Khaym03/falatracker/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
