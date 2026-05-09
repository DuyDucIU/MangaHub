package utils

import (
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"
)

// BindingError converts a gin binding/validation error into a human-readable string.
// Multiple field errors are joined with "; ".
func BindingError(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err.Error()
	}
	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		field := strings.ToLower(fe.Field())
		switch fe.Tag() {
		case "required":
			msgs = append(msgs, field+" is required")
		case "min":
			msgs = append(msgs, field+" must be at least "+fe.Param()+" characters")
		case "max":
			msgs = append(msgs, field+" must be at most "+fe.Param()+" characters")
		case "email":
			msgs = append(msgs, "email is invalid")
		default:
			msgs = append(msgs, field+" is invalid")
		}
	}
	return strings.Join(msgs, "; ")
}
