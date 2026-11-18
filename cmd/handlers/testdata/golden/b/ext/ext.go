package ext

type Token string

func (t Token) MarshalText() ([]byte, error)  { return []byte(t), nil }
func (t *Token) UnmarshalText(b []byte) error { *t = Token(b); return nil }
