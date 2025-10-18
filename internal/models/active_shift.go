// models/active_shift.go
package models

type ActiveShift struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	Username      string `json:"username"`
	StartTime     string `json:"start_time"`
	SlotTimeRange string `json:"slot_time_range"`
	Position      string `json:"position"`
	Zone          string `json:"zone"`
	SelfiePath    string `json:"selfie_path"`
}
