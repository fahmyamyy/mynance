package validate

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

var v = validator.New()

type Validatable interface {
	Validate() error
}

func Struct(s any) error {
	if err := v.Struct(s); err != nil {
		var errs validator.ValidationErrors
		if errors.As(err, &errs) {
			return fmt.Errorf("%s is %s", errs[0].Field(), errs[0].Tag())
		}
		return err
	}
	return nil
}
