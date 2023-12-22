package main

import (
	"context"
	"fmt"
	"os"
	"time"

	gtbxkfk "github.com/3rs4lg4d0/goutbox/emitter/kafka"
	"github.com/3rs4lg4d0/goutbox/gtbx"
	gtbxzrlg "github.com/3rs4lg4d0/goutbox/logger/zerolog"
	"github.com/3rs4lg4d0/goutbox/repository/pgxv5"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

func main() {
	p, _ := GetProducer()
	gtbx.Singleton(gtbx.Settings{
		EnableDispatcher: true,
	}, pgxv5.New(struct{}{}, GetDatabasePool()), gtbxkfk.New(p), gtbx.WithLogger(&gtbxzrlg.Logger{
		Logger: GetLogger(),
	}))

	<-time.After(time.Second * 300)

	fmt.Println("End!")
}

func GetLogger() zerolog.Logger {
	return zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}).
		Level(zerolog.Level(zerolog.DebugLevel)).
		With().
		Timestamp().
		Logger()
}

func GetProducer() (*kafka.Producer, error) {
	return kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  "localhost:19092",
		"linger.ms":          500,
		"batch.size":         100 * 1024,
		"compression.type":   "lz4",
		"acks":               -1,
		"enable.idempotence": true,
	})
}

func GetDatabasePool() *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig("postgresql://goutbox:goutbox@localhost:5432/goutbox?sslmode=disable")
	if err != nil {
		panic("Unable to parse database url")
	}
	db, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		panic("Unable to create connection pool")
	}
	return db
}
