package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"

	"github.com/devkekops/gophermart/internal/app/storage"
)

const (
	InvalidJSON          = "Invalid JSON"
	LoginAlreadyInUse    = "Login already in use"
	InternalServerError  = "Internal Server Error"
	InvalidCredentials   = "Invalid credentials"
	InvalidRequestFormat = "Invalid request format"
	InvalidOrderNumber   = "Invalid order number"
	NoOrders             = "No orders"
	NoWithdrawals        = "No withdrawals"
)

type Credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
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
	//fmt.Println(hex.EncodeToString(dst))

	sessionBytes := append(userIDBytes[:], dst[:]...)
	session := hex.EncodeToString(sessionBytes)
	//fmt.Println(session)

	return session
}

func (bh *BaseHandler) register() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var creds Credentials
		if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
			http.Error(w, InvalidJSON, http.StatusBadRequest)
			log.Println(err)
			return
		}

		passwordHash := getPasswordHash(creds.Password)
		//fmt.Println(passwordHash)

		userID, err := bh.repo.CreateUser(creds.Login, passwordHash)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
					http.Error(w, LoginAlreadyInUse, http.StatusConflict)
					log.Println(err)
					return
				}
				http.Error(w, InternalServerError, http.StatusInternalServerError)
				log.Println(err)
				return
			}
		}

		session := createSession(userID, bh.secretKey)
		//fmt.Println(session)
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
			http.Error(w, InvalidJSON, http.StatusBadRequest)
			log.Println(err)
			return
		}

		passwordHash := getPasswordHash(creds.Password)
		//fmt.Println(passwordHash)

		userID, err := bh.repo.AuthUser(creds.Login, passwordHash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, InvalidCredentials, http.StatusUnauthorized)
				log.Println(err)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
			return
		}
		//fmt.Println(userID)

		session := createSession(userID, bh.secretKey)
		//fmt.Println(session)

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
		userIDctx := req.Context().Value(userIDKey)
		userID := userIDctx.(string)

		b, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, InvalidRequestFormat, http.StatusBadRequest)
			log.Println(err)
			return
		}
		orderID := string(b)

		check, err := checkLuhn(orderID)
		if err != nil {
			http.Error(w, InvalidRequestFormat, http.StatusBadRequest)
			log.Println(err)
			return
		}

		if !check {
			http.Error(w, InvalidOrderNumber, http.StatusUnprocessableEntity)
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
				http.Error(w, InvalidRequestFormat, http.StatusBadRequest)
				log.Println(err)
				return
			}
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func (bh *BaseHandler) getOrders() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userIDctx := req.Context().Value(userIDKey)
		userID := userIDctx.(string)

		orders, err := bh.repo.GetOrders(userID)
		fmt.Println(orders)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
			return
		}

		if orders == nil {
			http.Error(w, NoOrders, http.StatusNoContent)
			return
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(orders)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	}
}

func (bh *BaseHandler) withdraw() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
	}
}

func (bh *BaseHandler) withdrawals() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userIDctx := req.Context().Value(userIDKey)
		userID := userIDctx.(string)

		withdrawals, err := bh.repo.GetWithdrawals(userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
			return
		}

		if withdrawals == nil {
			http.Error(w, NoWithdrawals, http.StatusNoContent)
			return
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(withdrawals)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	}
}
