package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"

	"github.com/devkekops/gophermart/internal/app/storage"
)

const (
	invalidJSON          = "Invalid JSON"
	loginAlreadyInUse    = "Login already in use"
	internalServerError  = "Internal Server Error"
	invalidCredentials   = "Invalid credentials"
	invalidRequestFormat = "Invalid request format"
	invalidOrderNumber   = "Invalid order number"
	noOrders             = "No orders"
	noWithdrawals        = "No withdrawals"
	insufficientFunds    = "Insuficient funds"
)

type Credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type Withdrawal struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

func getPasswordHash(password string) string {
	h := sha256.New()
	h.Write([]byte(password))
	hash := h.Sum(nil)
	passwordHash := hex.EncodeToString(hash)
	return passwordHash
}

func checkLuhn(orderID string) (bool, error) {
	number, err := strconv.Atoi(orderID)
	if err != nil {
		return false, err
	}
	payload := number / 10

	var luhn int
	for i := 0; payload > 0; i++ {
		cur := payload % 10
		if i%2 == 0 { // even
			cur = cur * 2
			if cur > 9 {
				cur = cur%10 + cur/10
			}
		}
		luhn += cur
		payload = payload / 10
	}
	checksum := luhn % 10

	return (number%10+checksum)%10 == 0, nil
}

func createSession(userID string, secretKey string) string {
	userIDBytes := []byte(userID)

	key := sha256.Sum256([]byte(secretKey))
	h := hmac.New(sha256.New, key[:])
	h.Write(userIDBytes)
	dst := h.Sum(nil)

	sessionBytes := append(userIDBytes[:], dst[:]...)
	session := hex.EncodeToString(sessionBytes)

	return session
}

func getUserID(req *http.Request) (string, error) {
	userIDctx := req.Context().Value(userIDKey)
	userID, ok := userIDctx.(string)
	if !ok {
		return "", errors.New("invalid userID in context")
	}
	return userID, nil
}

func (bh *BaseHandler) register() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var creds Credentials
		if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
			http.Error(w, invalidJSON, http.StatusBadRequest)
			log.Println(err)
			return
		}

		passwordHash := getPasswordHash(creds.Password)

		userID, err := bh.repo.CreateUser(creds.Login, passwordHash)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
					http.Error(w, loginAlreadyInUse, http.StatusConflict)
					log.Println(err)
					return
				}
				http.Error(w, internalServerError, http.StatusInternalServerError)
				log.Println(err)
				return
			}
		}

		session := createSession(userID, bh.secretKey)
		cookie := &http.Cookie{
			Name:  cookieName,
			Value: session,
			Path:  cookiePath,
		}
		http.SetCookie(w, cookie)

		w.WriteHeader(http.StatusOK)
	}
}

func (bh *BaseHandler) login() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var creds Credentials
		if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
			http.Error(w, invalidJSON, http.StatusBadRequest)
			log.Println(err)
			return
		}

		passwordHash := getPasswordHash(creds.Password)

		userID, err := bh.repo.AuthUser(creds.Login, passwordHash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, invalidCredentials, http.StatusUnauthorized)
				log.Println(err)
				return
			}
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		session := createSession(userID, bh.secretKey)

		cookie := &http.Cookie{
			Name:  cookieName,
			Value: session,
			Path:  cookiePath,
		}
		http.SetCookie(w, cookie)

		w.WriteHeader(http.StatusOK)
	}
}

func (bh *BaseHandler) loadOrder() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, err := getUserID(req)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		b, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, invalidRequestFormat, http.StatusBadRequest)
			log.Println(err)
			return
		}
		orderID := string(b)

		check, err := checkLuhn(orderID)
		if err != nil {
			http.Error(w, invalidRequestFormat, http.StatusBadRequest)
			log.Println(err)
			return
		}

		if !check {
			http.Error(w, invalidOrderNumber, http.StatusUnprocessableEntity)
			return
		}

		err = bh.repo.LoadOrder(orderID, userID)
		if err != nil {
			if errors.Is(err, storage.ErrOrderExistsForCurrentUser) {
				w.WriteHeader(http.StatusOK)
				log.Println(err)
				return
			} else if errors.Is(err, storage.ErrOrderExistsForOtherUser) {
				w.WriteHeader(http.StatusConflict)
				log.Println(err)
				return
			} else {
				http.Error(w, invalidRequestFormat, http.StatusBadRequest)
				log.Println(err)
				return
			}
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func (bh *BaseHandler) getOrders() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, err := getUserID(req)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		orders, err := bh.repo.GetOrders(userID)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		if orders == nil {
			http.Error(w, noOrders, http.StatusNoContent)
			return
		}

		buf, err := json.Marshal(orders)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(buf)
		if err != nil {
			log.Println(err)
		}
	}
}

func (bh *BaseHandler) getBalance() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, err := getUserID(req)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		balance, err := bh.repo.GetBalance(userID)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		buf, err := json.Marshal(balance)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(buf)
		if err != nil {
			log.Println(err)
		}
	}
}

func (bh *BaseHandler) withdraw() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, err := getUserID(req)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		var withdrawal Withdrawal
		if err := json.NewDecoder(req.Body).Decode(&withdrawal); err != nil {
			http.Error(w, invalidJSON, http.StatusBadRequest)
			log.Println(err)
			return
		}

		check, err := checkLuhn(withdrawal.Order)
		if err != nil {
			http.Error(w, invalidRequestFormat, http.StatusBadRequest)
			log.Println(err)
			return
		}

		if !check {
			http.Error(w, invalidOrderNumber, http.StatusUnprocessableEntity)
			return
		}

		err = bh.repo.Withdraw(withdrawal.Order, userID, withdrawal.Sum)
		if err != nil {
			if errors.Is(err, storage.ErrInsufficientFunds) {
				http.Error(w, insufficientFunds, http.StatusPaymentRequired)
				return
			} else {
				http.Error(w, internalServerError, http.StatusBadRequest)
				log.Println(err)
				return
			}
		}

		w.WriteHeader(http.StatusOK)

	}
}

func (bh *BaseHandler) withdrawals() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, err := getUserID(req)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		withdrawals, err := bh.repo.GetWithdrawals(userID)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		if withdrawals == nil {
			http.Error(w, noWithdrawals, http.StatusNoContent)
			return
		}

		buf, err := json.Marshal(withdrawals)
		if err != nil {
			http.Error(w, internalServerError, http.StatusInternalServerError)
			log.Println(err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(buf)
		if err != nil {
			log.Println(err)
		}
	}
}
