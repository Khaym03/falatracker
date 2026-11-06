package main

import (
	"errors"
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

var BrowserExecPathNotFoud = errors.New("browser exec path not found")

func NewBrowser() (*rod.Browser, error) {
	path, ok := launcher.LookPath()
	if !ok {
		return nil, BrowserExecPathNotFoud
	}

	fmt.Println("using path: ", path)
	u, err := launcher.New().
		Bin(path).
		Leakless(false).
		Headless(false).
		// Set("disable-popup-blocking", "true"). // Desactiva bloqueos extra o maneja comportamiento
		// Set("disable-notifications", "").      // Desactiva el popup de notificaciones
		// Set("disable-infobars", "").
		Launch()
	if err != nil {
		return nil, err
	}

	browser := rod.New().Trace(true).ControlURL(u)

	err = browser.Connect()
	if err != nil {
		return nil, err
	}

	return browser, nil
}
