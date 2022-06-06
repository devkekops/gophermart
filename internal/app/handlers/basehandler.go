package handlers

import (
	"github.com/devkekops/gophermart/internal/app/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type BaseHandler struct {
	mux       *chi.Mux
	secretKey string
	repo      storage.Repository
}

func NewBaseHandler(repo storage.Repository, secretKey string) *chi.Mux {
	bh := &BaseHandler{
		mux:       chi.NewMux(),
		secretKey: secretKey,
		repo:      repo,
	}

	bh.mux.Use(middleware.RequestID)
	bh.mux.Use(middleware.RealIP)
	bh.mux.Use(middleware.Logger)
	bh.mux.Use(middleware.Recoverer)

	bh.mux.Use(middleware.Compress(5))
	bh.mux.Use(gzipHandle)

	bh.mux.Route("/api/user", func(r chi.Router) {
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

	return bh.mux
}
