package ext

type CustomID string

func (c CustomID) MarshalText() ([]byte, error)  { return []byte(c), nil }
func (c *CustomID) UnmarshalText(b []byte) error { *c = CustomID(b); return nil }

type Slug string
