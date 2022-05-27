package storage

import (
	"context"
	"database/sql"
	"strconv"

	_ "github.com/jackc/pgx/v4/stdlib"
)

type RepoDB struct {
	db *sql.DB
}

func NewRepoDB(databaseURI string) (*RepoDB, error) {
	db, err := sql.Open("pgx", databaseURI)
	if err != nil {
		return nil, err
	}

	queryCreateTableUser := `
	CREATE TABLE IF NOT EXISTS users(
		user_id			SERIAL PRIMARY KEY,
		login			TEXT NOT NULL UNIQUE,
		password_hash	VARCHAR(64) NOT NULL,
		current			NUMERIC(15,2) NOT NULL DEFAULT 0.00,
		withdrawn		NUMERIC(15,2) NOT NULL DEFAULT 0.00
	);`

	queryCreateTableOrder := `
	CREATE TABLE IF NOT EXISTS orders(
		order_id		TEXT NOT NULL UNIQUE,
		user_id			INTEGER NOT NULL,
		status			VARCHAR(10) NOT NULL,
		accrual			NUMERIC(15,2) NOT NULL DEFAULT 0.00,
		uploaded_at		TIMESTAMP WITH TIME ZONE NOT NULL
	);`

	queryCreateTableWithdrawal := `
	CREATE TABLE IF NOT EXISTS withdrawals(
		order_id		TEXT NOT NULL,
		user_id			INTEGER NOT NULL,
		sum				NUMERIC(15,2) NOT NULL,
		processed_at	TIMESTAMP WITH TIME ZONE NOT NULL
	);`

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, queryCreateTableUser); err != nil {
		return nil, err
	}

	if _, err := db.ExecContext(ctx, queryCreateTableOrder); err != nil {
		return nil, err
	}

	if _, err := db.ExecContext(ctx, queryCreateTableWithdrawal); err != nil {
		return nil, err
	}

	r := &RepoDB{
		db: db,
	}

	return r, nil
}

func (r *RepoDB) CreateUser(login string, passwordHash string) error {
	querySaveUser := `INSERT INTO users (login, password_hash) VALUES ($1, $2)`
	if _, err := r.db.ExecContext(context.Background(), querySaveUser, login, passwordHash); err != nil {
		return err
	}
	return nil
}

func (r *RepoDB) AuthUser(login string, passwordHash string) (string, error) {
	queryAuthUser := `SELECT 1 FROM users WHERE login = ($1) AND password_hash = ($2)`
	var userID uint64
	row := r.db.QueryRowContext(context.Background(), queryAuthUser, login, passwordHash)
	if err := row.Scan(&userID); err != nil {
		return "", err
	}
	return strconv.FormatUint(userID, 10), nil
}

func (r *RepoDB) LoadOrder(orderID string, userID string) error {
	// 1. записывает в бд в таблицу order (order_id=orderID, user_id=userID, status=NEW, accrual=0, uploaded_at=time.Now())
	// 2. добавляет orderID в очередь на отправку в систему рассчёта
	// 3. workerpool в цикле проходит по очереди на отправку и шлёт запросы в систему через инициализированный в main.go клиент:
	// 		- при статус коде 200 - обновляет status, если status processed - обновляем accrual для заказа, удаляем из очереди, если другой - переместить в конец очереди
	//		- при статус коде 429 - переход к следующему элементу? можешь прийти не раньше чем через 5 секунд (сделать на channel-ах retry)
	return nil
}

func (r *RepoDB) GetOrders(userID string) ([]Order, error) {
	// Просто выводим содержимое таблицы order
	return nil, nil
}

func (r *RepoDB) GetBalance(userID string) (Balance, error) {
	return Balance{}, nil
}

func (r *RepoDB) Withdraw(orderID string, sum float64) error {
	return nil
}

func (r *RepoDB) GetWithdrawals(userID string) ([]Withdrawal, error) {
	var withdrawals []Withdrawal
	queryGetWithdrawals := "SELECT order_id, sum, processed_at FROM withdrawals WHERE user = ($1) ORDER BY processed_at"
	rows, err := r.db.QueryContext(context.Background(), queryGetWithdrawals, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var w Withdrawal
		err := rows.Scan(&w.OrderID, &w.Sum, &w.ProcessedAt)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return withdrawals, nil
}

func (r *RepoDB) Close() {
	r.db.Close()
}
