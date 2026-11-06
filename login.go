package main

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	if IsAlreadyLogedIn(ctx, page) {
		return nil
	}

	// Llenar usuario
	el, err := page.Element(userInputSelector)
	if err != nil {
		return fmt.Errorf("cannot find user input: %w", err)
	}
	if err = el.Input(cred.User); err != nil {
		return err
	}

	// Llenar contraseña
	el, err = page.Element(passwordInputSelector)
	if err != nil {
		return fmt.Errorf("cannot find password input: %w", err)
	}
	if err = el.Input(cred.Password); err != nil {
		return err
	}

	// Preparamos la espera de navegación
	waitNav := page.MustWaitNavigation()

	btn, err := page.Element(loginBtn)
	if err != nil {
		return fmt.Errorf("cannot find login button: %w", err)
	}

	if err = btn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("fail to click login button: %w", err)
	}

	// Esperamos a que termine la cadena de redirecciones del login
	waitNav()

	// ¡Listo! Ya estamos dentro del portal (en la página de bienvenida/dashboard).
	return nil
}

func IsAlreadyLogedIn(ctx context.Context, page *rod.Page) bool {
	if strings.Contains(page.MustInfo().URL, "fbusinesscenter.com/landing/") {
		return true
	}
	loginEl, err := page.Timeout(20 * time.Second).Element(`a[href="/login"]`)

	if err == nil {
		waitNav := page.MustWaitNavigation()
		loginEl.MustClick()
		waitNav()
		return true
	}

	return false
}
