package handler

import (
	"errors"

	"github.com/kilhog-io/kilhog/internal/service"
)

func errorMessage(err error, fallback string) string {
	var userErr *service.UserError
	if errors.As(err, &userErr) && userErr.Message != "" {
		return userErr.Message
	}
	return fallback
}
