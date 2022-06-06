package storage

import (
	"errors"
)

var ErrOrderExistsForCurrentUser = errors.New("order already been loaded by current user")
var ErrOrderExistsForOtherUser = errors.New("order already been loaded by other user")
var ErrInsufficientFunds = errors.New("insufficient funds")

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

type Repository interface {
	CreateUser(login string, passwordHash string) (string, error)
	AuthUser(login string, passwordHash string) (string, error)
	LoadOrder(orderID string, userID string) error
	GetOrders(userID string) ([]Order, error)
	GetBalance(userID string) (Balance, error)
	Withdraw(orderID string, userID string, sum float64) error
	GetWithdrawals(userID string) ([]Withdrawal, error)
	Close()
}
