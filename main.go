package main

import (
	"errors"
	"log"
)

func main() {
	browser, err := NewBrowser()
	if err != nil && errors.Is(err, BrowserExecPathNotFoud) {
		log.Fatal(err)
	}

	defer browser.MustClose()

}
