package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"

	"github.com/devkekops/gophermart/internal/app/client"
)

const (
	NEW        = "NEW"
	REGISTERED = "REGISTERED"
	INVALID    = "INVALID"
	PROCESSING = "PROCESSING"
	PROCESSED  = "PROCESSED"
)

type Worker struct {
	id    int
	repo  *RepoDB
	timer *time.Timer
}

func (w *Worker) loop() {
	// worker в цикле проходит по очереди на отправку и шлёт запросы в систему через инициализированный клиент:
	// 		- при статус коде 200 - обновляет status, если status PROCESSED или INVALID - обновляем accrual для заказа, удаляем из очереди,
	//			если REGISTERED - переместить в конец очереди, если PROCESSING - обновить статус и переместить в конец очереди
	//		- при статус коде 429 - можешь прийти не раньше чем через 5(или какое-то другое число) секунд (сделать на channel-ах retry)
	ctx := context.Background()
	queryUpdateOrderStatus := `UPDATE orders SET status = ($1) WHERE order_id = ($2)`
	queryUpdateOrderStatusAccrual := `UPDATE orders SET status = ($1), accrual = ($2) WHERE order_id = ($3) `
	for {
		<-w.timer.C
	ordersloop:
		for {
			currentOrder := <-w.repo.ordersCh
			_, err := w.repo.db.ExecContext(ctx, queryUpdateOrderStatus, NEW, currentOrder)
			if err != nil {
				log.Printf("error: %v\n", err)
			}

			accrualResp, err := w.repo.client.GetAccrualInfo(currentOrder)
			if err != nil {
				log.Printf("error: %v\n", err)
			}
			//fmt.Printf("worker #%d processed order %s, accrualResp: %+v\n", w.id, currentOrder, accrualResp)

			switch accrualResp.StatusCode {
			case 200:
				switch accrualResp.Status {
				case REGISTERED:
					w.repo.ordersCh <- currentOrder
				case PROCESSING:
					_, err := w.repo.db.ExecContext(ctx, queryUpdateOrderStatus, PROCESSING, currentOrder)
					if err != nil {
						log.Printf("error: %v\n", err)
					}
					w.repo.ordersCh <- currentOrder
				case INVALID, PROCESSED:
					_, err := w.repo.db.ExecContext(ctx, queryUpdateOrderStatusAccrual, accrualResp.Status, accrualResp.Accrual, currentOrder)
					if err != nil {
						log.Printf("error: %v\n", err)
					}
				}
			case 429:
				w.timer.Reset(10 * time.Second)
				break ordersloop
			case 500:
				w.repo.ordersCh <- currentOrder
			}
		}
	}
}

type RepoDB struct {
	db       *sql.DB
	client   *client.Client
	ordersCh chan string
}

func NewRepoDB(databaseURI string, client *client.Client) (*RepoDB, error) {
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
		accrual			NUMERIC(15,2),
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
		db:       db,
		client:   client,
		ordersCh: make(chan string),
	}

	workers := make([]*Worker, 0, runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		workers = append(workers, &Worker{i, r, time.NewTimer(0)})
	}

	for _, w := range workers {
		go w.loop()
	}

	return r, nil
}

func (r *RepoDB) CreateUser(login string, passwordHash string) (string, error) {
	var userID int64
	querySaveUser := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING user_id;`
	row := r.db.QueryRowContext(context.Background(), querySaveUser, login, passwordHash)
	if err := row.Scan(&userID); err != nil {
		return "", err
	}

	return strconv.FormatInt(userID, 10), nil
}

func (r *RepoDB) AuthUser(login string, passwordHash string) (string, error) {
	var userID int64
	queryAuthUser := `SELECT user_id FROM users WHERE login = ($1) AND password_hash = ($2)`
	row := r.db.QueryRowContext(context.Background(), queryAuthUser, login, passwordHash)
	if err := row.Scan(&userID); err != nil {
		return "", err
	}

	return strconv.FormatInt(userID, 10), nil
}

func (r *RepoDB) LoadOrder(orderID string, userID string) error {
	// 1. записывает в бд в таблицу order (order_id=orderID, user_id=userID, status=NEW, accrual=0, uploaded_at=time.Now())
	// 2. добавляет orderID в очередь на отправку в систему рассчёта

	var userIDExisting int64
	queryCheckIfOrderExists := `SELECT user_id FROM orders WHERE order_id = ($1)`
	row := r.db.QueryRowContext(context.Background(), queryCheckIfOrderExists, orderID)
	if err := row.Scan(&userIDExisting); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	uid := strconv.FormatInt(userIDExisting, 10)
	if uid == userID {
		return fmt.Errorf("%w", ErrOrderExistsForCurrentUser)
	} else if userIDExisting != 0 {
		return fmt.Errorf("%w", ErrOrderExistsForOtherUser)
	}

	querySaveNewOrder := `INSERT INTO orders (order_id, user_id, status, uploaded_at) VALUES ($1, $2, $3, $4)`
	_, err := r.db.ExecContext(context.Background(), querySaveNewOrder, orderID, userID, "NEW", time.Now().Truncate(time.Second))
	if err != nil {
		return err
	}

	go func() {
		r.ordersCh <- orderID
	}()

	return nil
}

func (r *RepoDB) GetOrders(userID string) ([]Order, error) {
	var orders []Order
	queryGetOrders := "SELECT order_id, status, accrual, uploaded_at FROM orders WHERE user_id = ($1) ORDER BY uploaded_at ASC"
	rows, err := r.db.QueryContext(context.Background(), queryGetOrders, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var o Order
		err := rows.Scan(&o.OrderID, &o.Status, &o.Accrual, &o.UploadedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return orders, nil
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
