package storage

import (
	"errors"

	"github.com/devkekops/gophermart/internal/app/entity"
)

var ErrOrderExistsForCurrentUser = errors.New("order already been loaded by current user")
var ErrOrderExistsForOtherUser = errors.New("order already been loaded by other user")
var ErrInsufficientFunds = errors.New("insufficient funds")

type Repository interface {
	CreateUser(login string, passwordHash string) (string, error)
	AuthUser(login string, passwordHash string) (string, error)
	LoadOrder(orderID string, userID string) error
	GetOrders(userID string) ([]entity.Order, error)
	GetBalance(userID string) (entity.Balance, error)
	Withdraw(orderID string, userID string, sum float64) error
	GetWithdrawals(userID string) ([]entity.Withdrawal, error)
	Close()
}
