package types

import "encoding/json"

type BasicAuthCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *BasicAuthCredentials) Serialise() ([]byte, error) {
	return json.Marshal(a)
}

func (a *BasicAuthCredentials) DeSerialise(data []byte) error {
	return json.Unmarshal(data, a)
}
