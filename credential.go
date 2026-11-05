package main

import (
	"fmt"
	"os"
)

type credential struct {
	User     string
	Password string
}

func GetCredentialFromENV() (credential, error) {
	c := credential{
		User:     os.Getenv("USER"),
		Password: os.Getenv("PASSWORD"),
	}

	if c.User == "" {
		return c, fmt.Errorf("USER env is not set")
	}

	if c.Password == "" {
		return c, fmt.Errorf("PASSWORD env is not set")
	}

	return c, nil
}
