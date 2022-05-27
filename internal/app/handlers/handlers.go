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
	"log"
	"net/http"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
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

func createSession(userID string, secretKey string) string {
	userIDBytes := []byte(userID)
	fmt.Println(hex.EncodeToString(userIDBytes))

	key := sha256.Sum256([]byte(secretKey))
	h := hmac.New(sha256.New, key[:])
	h.Write(userIDBytes)
	dst := h.Sum(nil)
	fmt.Println(hex.EncodeToString(dst))

	sessionBytes := append(userIDBytes[:], dst[:]...)
	session := hex.EncodeToString(sessionBytes)
	fmt.Println(session)

	return session
}

func (bh *BaseHandler) register() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var creds Credentials
		if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			log.Println(err)
			return
		}

		passwordHash := getPasswordHash(creds.Password)
		//fmt.Println(passwordHash)

		err := bh.repo.CreateUser(creds.Login, passwordHash)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
					http.Error(w, "Login already in use", http.StatusConflict)
					log.Println(err)
					return
				}
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				log.Println(err)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}
}

func (bh *BaseHandler) login() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var creds Credentials
		if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			log.Println(err)
			return
		}

		passwordHash := getPasswordHash(creds.Password)
		//fmt.Println(passwordHash)

		userID, err := bh.repo.AuthUser(creds.Login, passwordHash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
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

	}
}

func (bh *BaseHandler) getOrders() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
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
		fmt.Println(userID)

		withdrawals, err := bh.repo.GetWithdrawals(userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
			return
		}

		if withdrawals == nil {
			http.Error(w, "No withdrawals", http.StatusNoContent)
			return
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(withdrawals)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	}
}
