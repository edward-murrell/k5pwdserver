package model

type PasswordChangeRequest struct {
	Username    string `json:"username"`
	Realm       string `json:"realm"`
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}
