package main

import (
	"flag"
	"log"

	"github.com/caarlos0/env/v6"
	"github.com/devkekops/gophermart/internal/app/config"
	"github.com/devkekops/gophermart/internal/app/server"
)

func main() {
	cfg := config.Config{
		RunAddress:           "localhost:8081",
		DatabaseURI:          "postgres://localhost:5432/gophermart",
		AccrualSystemAddress: "localhost:8080",
		SecretKey:            "asdhkhk1375jwh132",
		ClientTimeout:        5,
	}

	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&cfg.RunAddress, "a", cfg.RunAddress, "run address")
	flag.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "database URI")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "accrual system address")
	flag.Parse()

	log.Fatal(server.Serve(&cfg))
}
