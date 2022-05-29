package handlers

import (
	"github.com/devkekops/gophermart/internal/app/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type BaseHandler struct {
	*chi.Mux
	secretKey string
	repo      storage.Repository
}

func NewBaseHandler(repo storage.Repository, secretKey string) *BaseHandler {
	bh := &BaseHandler{
		Mux:       chi.NewMux(),
		secretKey: secretKey,
		repo:      repo,
	}

	bh.Use(middleware.RequestID)
	bh.Use(middleware.RealIP)
	bh.Use(middleware.Logger)
	bh.Use(middleware.Recoverer)

	bh.Use(middleware.Compress(5))
	bh.Use(gzipHandle)

	bh.Route("/api/user", func(r chi.Router) {
		r.Post("/register", bh.register())
		r.Post("/login", bh.login())

		r.Group(func(r chi.Router) {
			r.Use(authHandle(bh.secretKey))
			r.Post("/orders", bh.loadOrder())
			r.Get("/orders", bh.getOrders())

			r.Route("/balance", func(r chi.Router) {
				r.Get("/", bh.getBalance())
				r.Post("/withdraw", bh.withdraw())
				r.Get("/withdrawals", bh.withdrawals())
			})
		})
	})

	return bh
}
