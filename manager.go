package main

import (
	"flag"
	"fmt"
	"path/filepath"
)

var (
	defaultDest = "/Users/sudeshpa/personal/photos/outcoming"
	defaultSrc  = "/Users/sudeshpa/personal/photos/incoming"
	printList   bool
	clearDB     bool
	serveMode   bool
)

func main() {
	flag.BoolVar(&printList, "print", false, "Print processed files at the end")
	flag.BoolVar(&clearDB, "clear-db", false, "Delete all records from incoming and outcoming tables and exit")
	flag.BoolVar(&serveMode, "serve", false, "Run HTTP API server and wait for requests")
	flag.Parse()

	if serveMode {
		dbPath := filepath.Join(defaultDest, "photoManager.db")
		if err := StartServer("127.0.0.1:7070", dbPath); err != nil {
			fmt.Println("server error:", err)
		}
		return
	}

	if clearDB {
		dbPath := filepath.Join(defaultDest, "photoManager.db")
		db, err := openAndInitDB(dbPath)
		if err != nil {
			fmt.Println("Failed to open DB:", err)
			return
		}
		defer db.Close()
		if err := db.clearDBTables(); err != nil {
			fmt.Println("Failed to clear DB:", err)
		} else {
			fmt.Println("Cleared DB tables: incoming, outcoming")
		}
		return
	}

	config := ProcessingConfig{
		SrcFolder:  defaultSrc,
		DestFolder: defaultDest,
	}
	fileProcessing(config)
	fmt.Println("File processing started in background")
}
