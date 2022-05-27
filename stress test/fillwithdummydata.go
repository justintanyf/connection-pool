package main

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"strconv"
)

func main() {
	fmt.Println(hashSHA256("admin"))
	db, err := sql.Open("mysql", "root:secret@tcp(localhost:3306)/db")
	if err != nil { // if there is an error opening the connection, handle it
		panic(err.Error())
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			log.Fatal("error closing connection to DB: ", err)
		}
	}(db)

	// Create a for loop of 200 that inserts the current iteration as both the account and the password
	for i := 147; i < 200; i++ {
		res, err := db.Query("INSERT INTO users (account, password) VALUES (?, ?)", strconv.Itoa(i), hashSHA256("test_password"))
		if err != nil {
			log.Fatal(err)
		}
		_ = res
	}
	fmt.Println("it works?")
}

func hashSHA256(stringToHash string) string {
	h := sha1.New()
	h.Write([]byte(stringToHash))
	sha1Hash := hex.EncodeToString(h.Sum(nil))
	return sha1Hash
}
