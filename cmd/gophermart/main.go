package main

import (
	"crypto/rand"
	"flag"
	"log"

	"github.com/caarlos0/env/v6"
	"github.com/devkekops/gophermart/internal/app/config"
	"github.com/devkekops/gophermart/internal/app/server"
)

func main() {
	randBytes := make([]byte, 16)
	_, err := rand.Read(randBytes)
	if err != nil {
		log.Fatal(err)
		return
	}
	secretKey := string(randBytes)

	cfg := config.Config{
		RunAddress:           "localhost:8081",
		DatabaseURI:          "postgres://localhost:5432/gophermart",
		AccrualSystemAddress: "http://localhost:8080",
		SecretKey:            secretKey,
		ClientTimeout:        5,
	}

	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
		return
	}

	flag.StringVar(&cfg.RunAddress, "a", cfg.RunAddress, "run address")
	flag.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "database URI")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "accrual system address")
	flag.StringVar(&cfg.SecretKey, "s", cfg.SecretKey, "secret key")
	flag.Parse()

	log.Fatal(server.Serve(&cfg))
}
