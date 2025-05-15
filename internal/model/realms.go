package model

type RealmsResponse struct {
	Realms       []string `json:"realms"`
	DefaultRealm string   `json:"default_realm"`
}
