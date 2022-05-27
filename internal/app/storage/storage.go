package storage

type Order struct {
	OrderID    string  `json:"number"`
	Status     string  `json:"status"`
	Accrual    float64 `json:"accrual"`
	UploadedAt string  `json:"uploaded_at"`
}

type Balance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type Withdrawal struct {
	OrderID     string  `json:"order"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

type Repository interface {
	CreateUser(login string, passwordHash string) error
	AuthUser(login string, passwordHash string) (string, error)
	LoadOrder(orderID string, userID string) error
	GetOrders(userID string) ([]Order, error)
	GetBalance(userID string) (Balance, error)
	Withdraw(orderID string, sum float64) error
	GetWithdrawals(userID string) ([]Withdrawal, error)
	Close()
}
