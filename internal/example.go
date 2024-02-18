package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	gtbxkfk "github.com/3rs4lg4d0/goutbox/emitter/kafka"
	"github.com/3rs4lg4d0/goutbox/gtbx"
	gtbxzrlg "github.com/3rs4lg4d0/goutbox/logger/zerolog"
	gtbxsql "github.com/3rs4lg4d0/goutbox/repository/sql"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"
)

func main() {
	p, _ := GetProducer()
	gtbx.Singleton(gtbx.Settings{
		EnableDispatcher: true,
	}, gtbxsql.New(struct{}{}, GetDatabasePool(), true), gtbxkfk.New(p), gtbx.WithLogger(&gtbxzrlg.Logger{
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

func GetDatabasePool() *sql.DB {
	db, err := sql.Open("pgx", "postgresql://goutbox:goutbox@localhost:5432/goutbox?sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	return db
}
