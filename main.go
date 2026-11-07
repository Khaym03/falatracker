package main

import (
	"context"
	"log"

	"github.com/go-rod/rod/lib/proto"
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

	ctx := context.Background()

	browser, err := NewBrowser()
	if err != nil {
		log.Fatal(err)
	}

	// defer browser.MustClose()

	pages, err := browser.Pages()
	if !pages.Empty() && len(pages) > 1 {
		for _, p := range pages[1:] {
			p.MustClose()
		}
	}

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		log.Fatal(err)
	}

	err = Login(ctx, page, cred)
	if err != nil {
		log.Fatal(err)
	}
	store := make(fakeStore)
	CollectOC(ctx, page, store)

}
