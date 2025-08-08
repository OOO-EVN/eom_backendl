package models

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	FirstName    string `json:"first_name,omitempty"`
	TelegramID   string `json:"telegram_id,omitempty"`
	Role         string `json:"role"`
	AvatarURL    string `json:"avatarUrl"`
	Email        string `json:"email,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Password  string `json:"password"`
	Email     string `json:"email,omitempty"`
}

type AuthResponse struct {
	Token    string `json:"token"`
	Role     string `json:"role"`
	UserID   int    `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

type TelegramAuthData struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
	AuthDate  string `json:"auth_date"`
	Hash      string `json:"hash"`
}
