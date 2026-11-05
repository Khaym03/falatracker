package main

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const loginURL = `https://access-key-corp.falabella.tech/auth/realms/esti/protocol/openid-connect/auth?client_id=portal&redirect_uri=https%3A%2F%2Ffbusinesscenter.com%2F&response_type=code&scope=openid&state=1739716db8fc4511b91fb1bf240805d7&code_challenge=nHB46AWINHfK5cOfiT5PyRQ7bDK5gZ70NiRMVUxCvjo&code_challenge_method=S256&fpkt=true`

const userInputSelector = `input[Id="username"]`
const passwordInputSelector = `input[Id="password"]`
const loginBtn = `form button`

func Login(ctx context.Context, page *rod.Page, cred credential) error {
	err := page.Navigate(loginURL)
	if err != nil {
		return fmt.Errorf("fail to navigate login: %s", loginURL)
	}

	if err = page.WaitLoad(); err != nil {
		return err
	}

	el, err := page.Element(userInputSelector)
	if err != nil {
		return fmt.Errorf("cannot find user input: %w", err)
	}

	if err = el.Input(cred.User); err != nil {
		return err
	}

	el, err = page.Element(passwordInputSelector)
	if err != nil {
		return fmt.Errorf("cannot find password input: %w", err)
	}

	if err = el.Input(cred.Password); err != nil {
		return err
	}

	el, err = page.Race().Element(loginBtn).Handle(func(e *rod.Element) error {
		return e.Click(proto.InputMouseButtonLeft, 1)
	}).Do()
	if err != nil {
		return fmt.Errorf("cannot find login button: %w", err)
	}

	return page.WaitLoad()
}
