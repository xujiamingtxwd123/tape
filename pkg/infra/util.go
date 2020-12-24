package infra

import (
	"github.com/satori/go.uuid"
)

func getuuid() (string, error) {
	u2, err := uuid.FromString("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	if err != nil {
		return "", nil
	}

	return u2.String(), nil
}
