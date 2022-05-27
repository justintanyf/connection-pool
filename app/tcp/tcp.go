package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	entrytaskproto "git.garena.com/wilber.chaowb/yanfeng-entry-task/protobuf_files/entry-task-proto"
	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"os"
	"strconv"
)

var db *sql.DB // Note the sql package provides the namespace
var redisDB *redis.Client
var ctx = context.Background()
var useCache bool

const (
	HOST             = "localhost"
	PORT             = "9001"
	TYPE             = "tcp"
	BufferHeaderSize = 4 // Because 4 bytes is an integer
)

/*
Buffer_header_meaning
0 -> Client wants to log in
1 -> Client wants to update nickname
2 -> Client wants to update image
3 -> request nickname and imagePath
*/
func handleIncomingRequest(conn net.Conn) {
	for {
		// Make 4 bytes on the buffer, read it, it will say for how long we need to read the input for
		buffer := make([]byte, BufferHeaderSize)
		_, err := conn.Read(buffer)

		if err != nil {
			log.Println("error reading buffer header from HTTP ", err)
		}
		messageLength := int(binary.LittleEndian.Uint32(buffer))

		buffer = make([]byte, messageLength)

		// read serialised information from the socket
		_, err = conn.Read(buffer)
		if err != nil {
			log.Println("error reading buffer header from HTTP ", err)
		}

		request := &entrytaskproto.Req{}
		if err := proto.Unmarshal(buffer, request); err != nil {
			log.Fatalln("Failed to parse buffer:", err)
		}
		if request.GetTypeOfMessage() == 0 {
			// 0 for requesting login
			loginProtobuf := &entrytaskproto.Login{}
			if err := proto.Unmarshal(request.GetPayload(), loginProtobuf); err != nil {
				log.Fatalln("Failed to parse payload:", err)
			}
			successfulLogin, id := attemptLogin(loginProtobuf.GetAccount(), loginProtobuf.GetPassword())
			if successfulLogin == 0 || successfulLogin == 1 || successfulLogin == -1 {
				// wrong, right, or unknown
				replyHTTPServer(conn, successfulLogin, id, "")
			} else {
				// something really bad happened so crash
				log.Fatal("Attempted login failed unexpectedly")
			}
		} else if request.GetTypeOfMessage() == 1 {
			// 1 for updating nickname
			updateNicknameProtobuf := &entrytaskproto.UpdateNickname{}
			if err := proto.Unmarshal(request.GetPayload(), updateNicknameProtobuf); err != nil {
				log.Fatalln("Failed to parse payload:", err)
			}
			successful := attemptUpdateNickname(int(updateNicknameProtobuf.GetId()), updateNicknameProtobuf.GetAccount(), updateNicknameProtobuf.GetNickname())
			if successful == 0 || successful == 1 || successful == -1 {
				// wrong, right, or unknown
				replyHTTPServer(conn, successful, -1, "")
			} else {
				// something really bad happened so crash
				log.Fatal("Attempted update nickname failed unexpectedly")
			}

		} else if request.GetTypeOfMessage() == 2 {
			// 2 for updating imagePath
			updateFileNameProtobuf := &entrytaskproto.UpdateFileName{}
			if err := proto.Unmarshal(request.GetPayload(), updateFileNameProtobuf); err != nil {
				log.Fatalln("Failed to parse payload:", err)
			}

			successful, oldFileName := attemptUpdateFilename(int(updateFileNameProtobuf.GetId()), updateFileNameProtobuf.GetAccount(), updateFileNameProtobuf.GetFileName())
			if successful == 0 || successful == 1 || successful == -1 {
				// wrong, right, or unknown
				replyHTTPServer(conn, successful, -1, oldFileName)
			} else {
				// something really bad happened so crash
				log.Fatal("Attempted update nickname failed unexpectedly")
			}

		} else if request.GetTypeOfMessage() == 3 {
			// 3 for updated nickname and filename
			GetNicknameAndFileNameProtobuf := &entrytaskproto.GetNicknameAndFileName{}
			if err := proto.Unmarshal(request.GetPayload(), GetNicknameAndFileNameProtobuf); err != nil {
				log.Fatalln("Failed to parse payload:", err)
			}
			nickname, fileName := getNicknameAndFileName(int(GetNicknameAndFileNameProtobuf.GetId()), GetNicknameAndFileNameProtobuf.GetAccount())
			response := &entrytaskproto.ReplyWithNicknameAndFileName{
				Nickname:  nickname,
				ImagePath: fileName,
			}
			responseSerialised, err := proto.Marshal(response)
			if err != nil {
				log.Fatal("error marshalling request", err)
			}
			// find length of responseSerialised
			lengthOfResponseSerialised := len(responseSerialised)
			// make buffer of size 4 bytes to hold an integer
			bufferHeader := make([]byte, 4)
			// store the integer value of the length to the buffer
			binary.LittleEndian.PutUint32(bufferHeader, uint32(lengthOfResponseSerialised))
			_, err = conn.Write(bufferHeader)
			if err != nil {
				log.Fatal("error writing buffer header from HTTP server to TCP server", err)
			}
			// write the serialised request to the TCP server
			_, err = conn.Write(responseSerialised)
			if err != nil {
				log.Fatal("error writing from HTTP server to TCP server", err)
			}
		} else {
			log.Fatal("unrecognised message")
		}
	}
}

func attemptLogin(account string, password string) (successfulLogin int, id int) {
	// Here I need to check redis
	// If hit, then check password, and return accordingly
	// if miss, use account to delete entry in redis, and use id to find
	if useCache {
		exists, err := redisDB.HExists(ctx, account, "password").Result()
		if err != nil {
			log.Fatal("error checking if key-value pair exists")
		} else if exists {
			// cache hit so find it
			passwordFromDB, err := redisDB.HGet(ctx, account, "password").Result()
			if err != nil {
				log.Fatal("cache hit but password field missing: ", err)
			}
			// compare password
			hashedPasswordInput := hashSHA256(password)
			if passwordFromDB != hashedPasswordInput {
				log.Println("wrong password")
				// need to redirect back to login
				return 0, -1 // 0 for false, wrong password
			}
			id, err := redisDB.HGet(ctx, account, "id").Result()
			if err != nil {
				log.Fatal("cache hit but password field missing: ", err)
			}
			idInt, err := strconv.Atoi(id)
			if err != nil {
				log.Fatal("error converting from string to integer: ", err)
			}
			return 1, idInt // 1 for success, correct account and password
		}
	}
	res, err := db.Query("SELECT id, nickname, password, pictureFileName FROM users where account=?", account)
	if err != nil {
		log.Fatal(err)
		// If there is an issue with the database, return a 500 error
		// w.WriteHeader(http.StatusInternalServerError)
		return -1, -1 // this means that a status 500 error should be returned
	}
	if !res.Next() {
		log.Println("no account found")
		return 0, -1 // 0 for false, no such account
	} else {
		var id int
		var nickname string
		var passwordFromDB string
		var pictureFileName string
		err := res.Scan(&id, &nickname, &passwordFromDB, &pictureFileName)
		if err != nil {
			log.Fatal(err)
		}

		if useCache {
			//set up the cache for that account now
			// Multiple field values for initializing Hash data
			value := make(map[string]interface{})
			value["id"] = id
			value["nickname"] = nickname
			value["password"] = passwordFromDB
			value["pictureFileName"] = pictureFileName

			// Save multiple Hash field values in one time
			err = redisDB.HMSet(ctx, account, value).Err()
			if err != nil {
				panic(err)
			}
		}

		hashedPasswordInput := hashSHA256(password)
		if passwordFromDB != hashedPasswordInput {
			log.Println("wrong password")
			// need to redirect back to login
			return 0, -1 // 0 for false, wrong password
		}
		return 1, id // 1 for success, correct account and password
	}
}

func attemptUpdateNickname(id int, account string, newNickname string) (successfulLogin int) {
	fmt.Println(id)
	fmt.Println(account)
	fmt.Println(newNickname)

	res, err := db.Exec("UPDATE users SET nickname=? WHERE id=?", newNickname, id)
	if err != nil {
		log.Fatal("issue with db so execute 500 error")
		// If there is an issue with the database, return a 500 error
		return -1
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Fatal(err)
		return 0
	}
	if rows != 1 {
		log.Fatalf("expected to affect 1 row, affected %d", rows)
		return 0
	}

	if useCache {
		err = redisDB.HSet(ctx, account, "nickname", newNickname).Err()
		if err != nil {
			log.Fatal("error in setting newNickname", err)
		}
	}
	return 1
}

func attemptUpdateFilename(id int, account string, newFileName string) (successfulLogin int, oldFileName string) {
	// find old name first
	res, err := db.Query("SELECT pictureFileName FROM users WHERE id=?", id)
	if err != nil {
		log.Fatal(err)
	}
	res.Next()
	err = res.Scan(&oldFileName)
	if err != nil {
		log.Fatal(err)
	}

	// update nickname now
	result, err := db.Exec("UPDATE users SET pictureFileName=? WHERE id=?", newFileName, id)
	if err != nil {
		log.Fatal("issue with db so execute 500 error")
		// If there is an issue with the database, return a 500 error
		return -1, ""
	}
	rows, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
		return 0, ""
	}
	if rows != 1 {
		log.Fatalf("expected to affect 1 row, affected %d", rows)
		return 0, ""
	}
	// write to cache new id
	if useCache {
		err = redisDB.HSet(ctx, account, "pictureFileName", newFileName).Err()
		if err != nil {
			log.Fatal("error in setting newFileName in cache", err)
		}
	}
	return 1, oldFileName
}

func getNicknameAndFileName(id int, account string) (string, string) {
	if useCache {
		// check if cache hit first
		exists, error := redisDB.Exists(ctx, account).Result()
		if error != nil {
			log.Fatal("error reading from cache")
		} else if exists != 0 {
			// cache hit
			nickname, error := redisDB.HGet(ctx, account, "nickname").Result()
			if error != nil {
				log.Fatal("error finding nickname", error)
			}
			pictureFileName, error := redisDB.HGet(ctx, account, "pictureFileName").Result()
			if error != nil {
				log.Fatal("error finding pictureFileName", error)
			}
			return nickname, pictureFileName
		}
	}

	res, err := db.Query("SELECT nickname, password, pictureFileName FROM users where id=?", id)
	if err != nil {
		log.Fatal(err)
	}
	res.Next()
	var nickname string
	var passwordFromDB string
	var pictureFileName string
	err = res.Scan(&nickname, &passwordFromDB, &pictureFileName)
	if err != nil {
		log.Fatal(err)
	}
	if useCache {
		//set up the cache for that account now
		// Multiple field values for initializing Hash data
		value := make(map[string]interface{})
		value["id"] = id
		value["nickname"] = nickname
		value["password"] = passwordFromDB
		value["pictureFileName"] = pictureFileName

		// Save multiple Hash field values in one time
		err = redisDB.HMSet(ctx, account, value).Err()
		if err != nil {
			panic(err)
		}
	}

	return nickname, pictureFileName
}

/*
status: 0 for fail, 1 for success
*/
func replyHTTPServer(conn net.Conn, status int, id int, oldFileName string) {
	response := &entrytaskproto.Response{
		Status:      int32(status),
		Id:          int32(id),
		OldFileName: oldFileName,
	}
	responseSerialised, err := proto.Marshal(response)
	if err != nil {
		log.Fatal("error marshalling request", err)
	}

	// find length of responseSerialised
	lengthOfResponseSerialised := len(responseSerialised)
	// make buffer of size 4 bytes to hold an integer
	bufferHeader := make([]byte, 4)
	// store the integer value of the length to the buffer
	binary.LittleEndian.PutUint32(bufferHeader, uint32(lengthOfResponseSerialised))
	_, err = conn.Write(bufferHeader)
	if err != nil {
		log.Fatal("error writing buffer header from HTTP server to TCP server", err)
	}
	// write the serialised request to the TCP server
	_, err = conn.Write(responseSerialised)
	if err != nil {
		log.Fatal("error writing from HTTP server to TCP server", err)
	}
}

func hashSHA256(stringToHash string) string {
	h := sha1.New()
	h.Write([]byte(stringToHash))
	sha1Hash := hex.EncodeToString(h.Sum(nil))
	return sha1Hash
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

func main() {

	//cpu profiler
	//flag.Parse()
	//if *cpuprofile != "" {
	//	f, err := os.Create(*cpuprofile)
	//	if err != nil {
	//		log.Fatal("could not create CPU profile: ", err)
	//	}
	//	defer f.Close()
	//	if err := pprof.StartCPUProfile(f); err != nil {
	//		log.Fatal("could not start CPU profile: ", err)
	//	}
	//	defer pprof.StopCPUProfile()
	//}
	//defer profile.Start(profile.ProfilePath(".")).Stop()

	// use cache or not
	if len(os.Args) > 1 {
		if os.Args[2] == "y" || os.Args[2] == "yes" {
			useCache = true
		} else if os.Args[2] == "n" || os.Args[2] == "no" {
			useCache = false
		} else {
			log.Fatal("Unknown input, please enter y/yes or n/no")
		}
	}

	// connect to DB
	var err error
	db, err = sql.Open("mysql", "root:secret@tcp(localhost:3306)/db")
	if err != nil { // if there is an error opening the connection, handle it
		panic(err.Error())
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			log.Fatal("error closing connection to DB: ", err)
		}
	}(db)

	listen, err := net.Listen(TYPE, HOST+":"+PORT)
	if err != nil {
		log.Fatal(err)
	}
	// close listener
	defer func(listen net.Listener) {
		err := listen.Close()
		if err != nil {
			log.Fatal("error closing listener", err)
		}
	}(listen)

	if useCache {
		// Implement redis here
		redisDB = redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		})
		pong, err := redisDB.Ping(ctx).Result()
		fmt.Println(pong, err)
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleIncomingRequest(conn)
	}

	// memory profiler
	//if *memprofile != "" {
	//	f, err := os.Create(*memprofile)
	//	if err != nil {
	//		log.Fatal("could not create memory profile: ", err)
	//	}
	//	defer f.Close()
	//	runtime.GC() // get up-to-date statistics
	//	if err := pprof.WriteHeapProfile(f); err != nil {
	//		log.Fatal("could not write memory profile: ", err)
	//	}
	//}
}
