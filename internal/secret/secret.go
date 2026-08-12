// Package secret stores the SMTP password in the OS keychain.
package secret

import (
	"github.com/zalando/go-keyring"
)

const service = "outis"

func Set(username, password string) error {
	return keyring.Set(service, username, password)
}

func Get(username string) (string, error) {
	return keyring.Get(service, username)
}

func Delete(username string) error {
	return keyring.Delete(service, username)
}
