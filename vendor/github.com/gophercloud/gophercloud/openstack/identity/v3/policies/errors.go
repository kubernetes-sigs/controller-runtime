package policies

import "fmt"

// StringFieldLengthExceedsLimit xxx
type StringFieldLengthExceedsLimit struct {
	Field string
	Limit int
}

func (e StringFieldLengthExceedsLimit) Error() string {
	return fmt.Sprintf("String length of field [%s] exceeds limit (%d)",
		e.Field, e.Limit,
	)
}
