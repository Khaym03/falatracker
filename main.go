package main

import (
	"log"

	"github.com/joho/godotenv"
)

var user string
var password string

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	cred, err := GetCredentialFromENV()
	if err != nil {
		log.Fatal(err)
	}

	browser, err := NewBrowser()
	if err != nil {
		log.Fatal(err)
	}

	defer browser.MustClose()

}
