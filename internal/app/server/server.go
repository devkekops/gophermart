package server

import (
	"net/http"

	"github.com/devkekops/gophermart/internal/app/client"
	"github.com/devkekops/gophermart/internal/app/config"
	"github.com/devkekops/gophermart/internal/app/handlers"
	"github.com/devkekops/gophermart/internal/app/logger"
	"github.com/devkekops/gophermart/internal/app/storage"
)

func Serve(cfg *config.Config) error {
	client := client.NewCli(cfg.AccrualSystemAddress, cfg.ClientTimeout)

	repo, err := storage.NewRepoDB(cfg.DatabaseURI, client)
	if err != nil {
		logger.Logger.Err(err).Msg("")
	}
	defer repo.Close()

	var baseHandler = handlers.NewBaseHandler(repo, cfg.SecretKey)

	server := &http.Server{
		Addr:    cfg.RunAddress,
		Handler: baseHandler,
	}

	return server.ListenAndServe()
}
