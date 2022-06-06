package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/devkekops/gophermart/internal/app/client"
	"github.com/devkekops/gophermart/internal/app/entity"
)

const (
	NEW        = "NEW"
	REGISTERED = "REGISTERED"
	INVALID    = "INVALID"
	PROCESSING = "PROCESSING"
	PROCESSED  = "PROCESSED"
)

var schema = `
CREATE TABLE IF NOT EXISTS users(
	user_id			SERIAL PRIMARY KEY,
	login			TEXT NOT NULL UNIQUE,
	password_hash	VARCHAR(64) NOT NULL,
	current			NUMERIC(15,2) NOT NULL DEFAULT 0.00,
	withdrawn		NUMERIC(15,2) NOT NULL DEFAULT 0.00
);

CREATE TABLE IF NOT EXISTS orders(
	order_id		TEXT NOT NULL UNIQUE,
	user_id			INTEGER NOT NULL,
	status			VARCHAR(10) NOT NULL,
	accrual			NUMERIC(15,2) NOT NULL DEFAULT 0.00,
	uploaded_at		TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS withdrawals(
	order_id		TEXT NOT NULL,
	user_id			INTEGER NOT NULL,
	sum				NUMERIC(15,2) NOT NULL,
	processed_at	TIMESTAMP WITH TIME ZONE NOT NULL
);`

type Task struct {
	userID  string
	orderID string
}

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
	queryUpdateOrderStatus := `UPDATE orders SET status = ($1) WHERE order_id = ($2)`
	queryUpdateOrderStatusAccrual := `UPDATE orders SET status = ($1), accrual = ($2) WHERE order_id = ($3)`
	queryUpdateUserCurrent := `UPDATE users SET current = current + ($1) WHERE user_id = ($2)`

	for {
		<-w.timer.C
	taskloop:
		for {
			task := <-w.repo.taskCh
			_, err := w.repo.db.Exec(queryUpdateOrderStatus, NEW, task.orderID)
			if err != nil {
				log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
			}

			accrualResp, err := w.repo.client.GetAccrualInfo(task.orderID)
			if err != nil {
				log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
			}
			fmt.Printf("worker #%d processed order %s, accrualResp: %v\n", w.id, task.orderID, accrualResp)

			switch accrualResp.StatusCode {
			case 200:
				switch accrualResp.Status {
				case REGISTERED:
					w.repo.taskCh <- task

				case PROCESSING:
					_, err := w.repo.db.Exec(queryUpdateOrderStatus, PROCESSING, task.orderID)
					if err != nil {
						log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
					}
					w.repo.taskCh <- task

				case INVALID:
					_, err := w.repo.db.Exec(queryUpdateOrderStatus, INVALID, task.orderID)
					if err != nil {
						log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
					}

				case PROCESSED:
					tx, err := w.repo.db.Begin()
					if err != nil {
						log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
					}
					defer func(tx *sql.Tx) {
						err := tx.Rollback()
						if err != nil {
							log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
						}
					}(tx)

					_, err = tx.Exec(queryUpdateOrderStatusAccrual, PROCESSED, accrualResp.Accrual, task.orderID)
					if err != nil {
						log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
					}
					_, err = tx.Exec(queryUpdateUserCurrent, accrualResp.Accrual, task.userID)
					if err != nil {
						log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
					}
					err = tx.Commit()
					if err != nil {
						log.Printf("worker #%d task #%v error: %v\n", w.id, task, err)
					}
				}
			case 429:
				w.timer.Reset(10 * time.Second)
				break taskloop
			case 500:
				w.repo.taskCh <- task
			}
		}
	}
}

type RepoDB struct {
	db     *sqlx.DB
	client client.Client
	taskCh chan *Task
}

func NewRepoDB(databaseURI string, client client.Client) (*RepoDB, error) {
	db, err := sqlx.Connect("pgx", databaseURI)
	if err != nil {
		return nil, err
	}

	db.MustExec(schema)

	r := &RepoDB{
		db:     db,
		client: client,
		taskCh: make(chan *Task),
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
	err := r.db.Get(&userID, querySaveUser, login, passwordHash)
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(userID, 10), nil
}

func (r *RepoDB) AuthUser(login string, passwordHash string) (string, error) {
	var userID int64
	queryAuthUser := `SELECT user_id FROM users WHERE login = ($1) AND password_hash = ($2)`
	err := r.db.Get(&userID, queryAuthUser, login, passwordHash)
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(userID, 10), nil
}

func (r *RepoDB) LoadOrder(orderID string, userID string) error {
	// 1. записывает в бд в таблицу order (order_id=orderID, user_id=userID, status=NEW, accrual=0, uploaded_at=time.Now())
	// 2. добавляет новую задачу с {userID, orderID} в очередь на отправку в систему рассчёта
	var userIDExisting int64
	queryCheckIfOrderExists := `SELECT user_id FROM orders WHERE order_id = ($1)`
	err := r.db.Get(&userIDExisting, queryCheckIfOrderExists, orderID)
	if err != nil {
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
	_, err = r.db.Exec(querySaveNewOrder, orderID, userID, "NEW", time.Now().Truncate(time.Second))
	if err != nil {
		return err
	}

	go func() {
		r.taskCh <- &Task{userID, orderID}
	}()

	return nil
}

func (r *RepoDB) GetOrders(userID string) ([]entity.Order, error) {
	var orders []entity.Order
	queryGetOrders := "SELECT order_id, status, accrual, uploaded_at FROM orders WHERE user_id = ($1) ORDER BY uploaded_at ASC"
	err := r.db.Select(&orders, queryGetOrders, userID)
	if err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *RepoDB) GetBalance(userID string) (entity.Balance, error) {
	var balance entity.Balance
	queryGetBalance := `SELECT current, withdrawn FROM users WHERE user_id = ($1)`
	err := r.db.Get(&balance, queryGetBalance, userID)
	if err != nil {
		return balance, err
	}
	return balance, nil
}

func (r *RepoDB) Withdraw(orderID string, userID string, sum float64) error {
	queryUpdateUserBalance := `UPDATE users SET current = current - ($1), withdrawn = withdrawn + ($1) WHERE user_id = ($2) RETURNING current`
	queryAddWithdraw := `INSERT INTO withdrawals (order_id, user_id, sum, processed_at) VALUES ($1, $2, $3, $4)`

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func(tx *sql.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Println(err)
		}
	}(tx)

	var newBalance float64
	err = tx.QueryRow(queryUpdateUserBalance, sum, userID).Scan(&newBalance)
	if err != nil {
		return err
	}
	if newBalance < 0 {
		return ErrInsufficientFunds
	}

	_, err = tx.Exec(queryAddWithdraw, orderID, userID, sum, time.Now().Truncate(time.Second))
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (r *RepoDB) GetWithdrawals(userID string) ([]entity.Withdrawal, error) {
	var withdrawals []entity.Withdrawal
	queryGetWithdrawals := "SELECT order_id, sum, processed_at FROM withdrawals WHERE user_id = ($1) ORDER BY processed_at ASC"

	err := r.db.Select(&withdrawals, queryGetWithdrawals, userID)
	if err != nil {
		return nil, err
	}

	return withdrawals, nil
}

func (r *RepoDB) Close() {
	r.db.Close()
}
