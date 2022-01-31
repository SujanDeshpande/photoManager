package postgres

import (
	"fmt"
	"log"
	"time"
)

type Photostore struct {
	id        int       `json:"id"`
	fileName  string    `json:"fileName"`
	createdBy string    `json:"createdBy"`
	createdAt time.Time `json:"createdAt"`
	updatedBy string    `json:"updatedBy"`
	updatedAt time.Time `json:"updatedAt"`
}

func NewPhotoStore() *Photostore {
	return &Photostore{
		createdAt: time.Now(),
		updatedAt: time.Now(),
		createdBy: "sujan",
		updatedBy: "sujan",
		fileName:  "temp",
	}
}

func ReadAll() ([]Photostore, error) {
	tsql := fmt.Sprintf("select * from photostore;")
	rows, err := DB.Query(tsql)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	count := 0
	got := []Photostore{}
	for rows.Next() {
		var ps Photostore
		rows.Scan(&ps.id, &ps.fileName, &ps.createdBy, &ps.createdAt, &ps.updatedBy, &ps.updatedAt)
		got = append(got, ps)
		count++
	}
	return got, nil
}

func Insert(ps *Photostore) (int, error) {
	tsql := fmt.Sprintf("INSERT INTO photostore (file_name, created_by, created_at, updated_by, updated_at) VALUES ($1, $2, $3, $4, $5) RETURNING id")
	var id int

	// execute the sql statement
	// Scan function will save the insert id in the id
	err := DB.QueryRow(tsql, ps.fileName, ps.createdBy, ps.createdAt, ps.updatedBy, ps.updatedAt).Scan(&id)
	if err != nil {
		log.Fatalf("Unable to execute the query. %v", err)
	}

	fmt.Printf("Inserted a single record %v", id)

	// return the inserted id
	return id, nil
}

func Update(id int, fileName string) (int, error) {
	tsql := fmt.Sprintf("UPDATE photostore SET file_name=$2 WHERE id=$1")

	// execute the sql statement
	// Scan function will save the insert id in the id
	_, err := DB.Exec(tsql, id, fileName)
	if err != nil {
		log.Fatalf("Unable to execute the query. %v", err)
	}

	fmt.Printf("Updated a single record %v", id)

	return id, nil
}

func Delete(id int) (int, error) {
	tsql := fmt.Sprintf("DELETE FROM photostore WHERE id=$1")
	_, err := DB.Exec(tsql, id)
	if err != nil {
		log.Fatalf("Unable to execute the query. %v", err)
	}
	return id, nil
}
