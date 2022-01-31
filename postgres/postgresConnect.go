package postgres

import (
	"database/sql"
	"fmt"
	"photoManager/utils"

	_ "github.com/lib/pq"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "narvar"
	password = "narvar"
	dbname   = "photoDB"
)

var DB *sql.DB

var (
	id        int
	fileName  string
	createdBy string
	createdAt string
	updatedBy string
	updatedAt string
)

func InitializePostgres() {
	psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlconn)
	utils.CheckError(err)
	// check db
	err = db.Ping()
	utils.CheckError(err)

	fmt.Println("Connected Postgres!")
	DB = db
	go utils.Quit("Postgres", Close)
}

func Close() {
	DB.Close()
}
