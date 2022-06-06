package entity

type Order struct {
	OrderID    string  `json:"number" db:"order_id"`
	Status     string  `json:"status" db:"status"`
	Accrual    float64 `json:"accrual,omitempty" db:"accrual"`
	UploadedAt string  `json:"uploaded_at" db:"uploaded_at"`
}

type Balance struct {
	Current   float64 `json:"current" db:"current"`
	Withdrawn float64 `json:"withdrawn" db:"withdrawn"`
}

type Withdrawal struct {
	OrderID     string  `json:"order" db:"order_id"`
	Sum         float64 `json:"sum" db:"sum"`
	ProcessedAt string  `json:"processed_at" db:"processed_at"`
}
