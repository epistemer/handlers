package ids

import "errors"

// OrderID is a typed integer identifier for an order.
type OrderID int64

// Token is a typed text identifier implementing TextMarshaler/TextUnmarshaler.
type Token struct {
	Raw string
}

// MarshalText encodes t.
func (t Token) MarshalText() ([]byte, error) {
	return []byte(t.Raw), nil
}

// UnmarshalText decodes b into t, rejecting empty or "bad" values.
func (t *Token) UnmarshalText(b []byte) error {
	if len(b) == 0 || string(b) == "bad" {
		return errors.New("invalid token")
	}
	t.Raw = string(b)
	return nil
}
