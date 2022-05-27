package server

import (
	"log"
	"net/http"

	"github.com/devkekops/gophermart/internal/app/config"
	"github.com/devkekops/gophermart/internal/app/handlers"
	"github.com/devkekops/gophermart/internal/app/storage"
)

func Serve(cfg *config.Config) error {
	repo, err := storage.NewRepoDB(cfg.DatabaseURI)
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()

	var baseHandler = handlers.NewBaseHandler(repo, cfg.SecretKey)

	server := &http.Server{
		Addr:    cfg.RunAddress,
		Handler: baseHandler,
	}

	return server.ListenAndServe()
}
