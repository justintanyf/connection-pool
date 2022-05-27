/*
HTTP server is to open and handle HTML connections, receive information from client-side and notify TCP server, take information from TCP server, and decide how to render HTML
*/

package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	entrytaskproto "git.garena.com/wilber.chaowb/yanfeng-entry-task/protobuf_files/entry-task-proto"
	"github.com/dgrijalva/jwt-go"
	"google.golang.org/protobuf/proto"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var secretKey = []byte("my_secret_key")

// tcpConn is a wrapper for a single tcp connection
type tcpConn struct {
	id         string   // A unique id to identify a connection
	connection net.Conn // The underlying TCP connection
}

type TCPConnectionPool struct {
	mu   sync.Mutex
	pool chan tcpConn
}

var connectionPool *TCPConnectionPool

type Claims struct {
	Id      int    `json:"id"`
	Account string `json:"account"`
	jwt.StandardClaims
}

const (
	HOST             = "localhost"
	PORT             = "9001"
	TYPE             = "tcp"
	BufferHeaderSize = 4 // Because 4 bytes is an integer
)

func login(w http.ResponseWriter, r *http.Request) {
	log.Println("Method for login: ", r.Method) //get request method
	if r.Method == "GET" {
		// check the token here if it exists
		if !checkValidCookie(w, r) {
			fmt.Println("invalid cookie")
			t, _ := template.ParseFiles("./HTML_Pages/login.gtpl")
			t.Execute(w, nil)
			return
		} else {
			// token is correct and not expired so go to userpage
			http.Redirect(w, r, "/userpage", http.StatusFound)
			return
		}
	} else {
		r.ParseForm()
		fmt.Println("account:", r.Form["account"])
		fmt.Println("password:", r.Form["password"])

		login := &entrytaskproto.Login{
			Account:  strings.Join(r.Form["account"], ""),
			Password: strings.Join(r.Form["password"], ""),
		}
		payload, err := proto.Marshal(login)
		if err != nil {
			log.Fatal("error marshalling login", err)
		}
		buffer := sendPayloadAndReceiveBuffer(0, payload) // 0 for login
		reply := &entrytaskproto.Response{}
		if err := proto.Unmarshal(buffer, reply); err != nil {
			log.Fatalln("Failed to parse reply from TCP: ", err)
		}
		if reply.GetStatus() == 0 {
			//not correct so redirect
			fmt.Println("not found")
			// need to redirect back to login
			http.Redirect(w, r, "/", http.StatusFound)
		} else if reply.GetStatus() == 1 {
			// generate jwt token and issue it here
			// Declare the expiration time of the token
			// here, we have kept it as 5 minutes
			expirationTime := time.Now().Add(5 * time.Minute)
			// Create the JWT claims, which includes the username and expiry time
			claims := &Claims{
				// add id here
				Id:      int(reply.GetId()),
				Account: r.FormValue("account"),
				StandardClaims: jwt.StandardClaims{
					// In JWT, the expiry time is expressed as unix milliseconds
					ExpiresAt: expirationTime.Unix(),
				},
			}

			// Declare the token with the algorithm used for signing, and the claims
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			// Create the JWT string
			tokenString, err := token.SignedString(secretKey)
			if err != nil {
				// If there is an error in creating the JWT return an internal server error
				fmt.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// Finally, we set the client cookie for "token" as the JWT we just generated
			// we also set an expiry time which is the same as the token itself
			http.SetCookie(w, &http.Cookie{
				Name:    "token",
				Value:   tokenString,
				Expires: expirationTime,
			})
			http.Redirect(w, r, "/userpage", http.StatusFound)
		} else if reply.GetStatus() == -1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		} else {
			log.Fatal("unknown status reply from tcp")
		}
	}
}

func sendPayloadAndReceiveBuffer(typeOfMessage int, payload []byte) (bufferWithResponse []byte) {
	request := &entrytaskproto.Req{
		TypeOfMessage: int32(typeOfMessage),
		Payload:       payload,
	}
	// Generate the bytespace to hold the size of thing
	requestBytes, err := proto.Marshal(request)
	if err != nil {
		log.Fatal("error marshalling request", err)
	}

	// find length of requestBytes
	lengthOfRequestBytes := len(requestBytes)
	// make buffer of size 4 bytes to hold an integer
	bufferHeader := make([]byte, BufferHeaderSize)
	// store the integer value of the length to the bufferHeader
	binary.LittleEndian.PutUint32(bufferHeader, uint32(lengthOfRequestBytes))
	conn := get()
	_, err = conn.connection.Write(bufferHeader)
	if err != nil {
		log.Fatal("error writing buffer header from HTTP server to TCP server", err)
	}
	// write the serialised request to the TCP server
	_, err = conn.connection.Write(requestBytes)
	if err != nil {
		log.Fatal("error writing from HTTP server to TCP server", err)
	}

	// Parse reply from TCP server
	buffer := make([]byte, BufferHeaderSize)
	_, err = conn.connection.Read(buffer)
	if err != nil {
		fmt.Println("error reading buffer header from TCP ", err)
	}
	messageLength := int(binary.LittleEndian.Uint32(buffer))

	buffer = make([]byte, messageLength)

	// read serialised information from the socket
	_, err = conn.connection.Read(buffer)
	if err != nil {
		fmt.Println("error reading serialised information from TCP ", err)
	}
	release(conn)
	return buffer
}

func hashSHA256(stringToHash string) string {
	h := sha1.New()
	h.Write([]byte(stringToHash))
	sha1Hash := hex.EncodeToString(h.Sum(nil))
	return sha1Hash
}

func userpage(w http.ResponseWriter, r *http.Request) {
	log.Println("Method for userpage: ", r.Method) //get request method
	if !checkValidCookie(w, r) {
		fmt.Println("invalid cookie so go back to login")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if r.Method == "GET" {
		nickname, fileName := getNicknameAndFileName(r)
		t, _ := template.ParseFiles("HTML_Pages/userpage.gtpl")

		// find file and insert into HTML
		relativeFilePath := "Images/" + fileName
		fmt.Fprintf(w, "<html><img src=\""+relativeFilePath+"\" alt='no image set yet' style='width:235px;height:320px;'></html>")
		t.ExecuteTemplate(w, "userpage", nickname)
	} else {
		r.ParseForm()
		// logic part of updating MySQL table
		id, account := getIdAndAccountName(r)
		if r.Form["nickname"] != nil {
			updateNicknameProto := &entrytaskproto.UpdateNickname{
				Id:       int32(id),
				Account:  account,
				Nickname: strings.Join(r.Form["nickname"], ""),
			}
			payload, err := proto.Marshal(updateNicknameProto)
			if err != nil {
				log.Fatal("error marshalling updateNicknameProto", err)
			}

			buffer := sendPayloadAndReceiveBuffer(1, payload) // 1 for updating nickname
			response := &entrytaskproto.Response{}
			if err := proto.Unmarshal(buffer, response); err != nil {
				log.Fatalln("Failed to parse reply from TCP: ", err)
			}
			if response.GetStatus() == 1 {
				// success so redirect
				http.Redirect(w, r, "/userpage", http.StatusFound)
			} else if response.GetStatus() == 0 {
				log.Fatal("failed to update nickname")
			} else {
				log.Fatal("unexpected error")
			}
		}
	}
}

func getIdAndAccountName(r *http.Request) (id int, account string) {
	c, err := r.Cookie("token") // Get the JWT string from the cookie
	if err != nil {
		log.Fatal("Error reading token for some reason: ", err)
	}
	tknStr := c.Value

	// Initialize a new instance of `Claims`
	claims := &Claims{}

	// Parse the JWT string and store the result in `claims`.
	// Note that we are passing the key in this method as well. This method will return an error
	// if the token is invalid (if it has expired according to the expiry time we set on sign in),
	// or if the signature does not match
	tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})
	claims, ok := tkn.Claims.(*Claims)
	if !ok || !tkn.Valid {
		fmt.Println("there was an error making the claims")
		fmt.Println(err)
	}
	return claims.Id, claims.Account
}

func getNicknameAndFileName(r *http.Request) (nickname string, fileName string) {
	id, account := getIdAndAccountName(r)
	getNicknameandFileNameProto := &entrytaskproto.GetNicknameAndFileName{
		Id:      int32(id),
		Account: account,
	}
	payload, err := proto.Marshal(getNicknameandFileNameProto)
	if err != nil {
		log.Fatal("error marshalling getNicknameandFileNameProto", err)
	}

	buffer := sendPayloadAndReceiveBuffer(3, payload) // 3 for get nickname and filename
	replyWithNicknameAndFileName := &entrytaskproto.ReplyWithNicknameAndFileName{}
	if err := proto.Unmarshal(buffer, replyWithNicknameAndFileName); err != nil {
		log.Fatalln("Failed to parse reply from TCP: ", err)
	}
	return replyWithNicknameAndFileName.GetNickname(), replyWithNicknameAndFileName.GetImagePath()
}

func checkValidCookie(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie("token")
	if err != nil {
		if err == http.ErrNoCookie {
			log.Println("no token so login again")
			return false
		}
		// For any other type of error, return a bad request status
		log.Println("bad request")
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	// Get the JWT string from the cookie
	tknStr := c.Value

	// Initialize a new instance of `Claims`
	claims := &Claims{}

	// Parse the JWT string and store the result in `claims`.
	// Note that we are passing the key in this method as well. This method will return an error
	// if the token is invalid (if it has expired according to the expiry time we set on sign in),
	// or if the signature does not match
	tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})
	if err != nil {
		if err == jwt.ErrSignatureInvalid {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	if !tkn.Valid {
		// expired token so login again
		return false
	}
	return true
}

func uploadImage(w http.ResponseWriter, r *http.Request) {
	log.Println("Method for uploadImage: ", r.Method) //get request method
	if !checkValidCookie(w, r) {
		fmt.Println("invalid cookie so go back to login")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if r.Method == "GET" {
		http.Redirect(w, r, "/userpage", http.StatusFound)
	} else {

		r.ParseMultipartForm(32 << 20)
		file, handler, err := r.FormFile("image")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
		// has file name with a random number, store it with the file extension
		fileExtension := filepath.Ext(handler.Filename)
		handler.Filename = hashSHA256(time.Now().String()+string(rand.Int63())) + fileExtension
		f, err := os.OpenFile("./Images/"+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer f.Close()
		io.Copy(f, file)

		// Delete previous copy
		id, account := getIdAndAccountName(r)

		updateFileNameProto := &entrytaskproto.UpdateFileName{
			Id:       int32(id),
			Account:  account,
			FileName: handler.Filename,
		}
		payload, err := proto.Marshal(updateFileNameProto)
		if err != nil {
			log.Fatal("error marshalling updateFileNameProto", err)
		}

		buffer := sendPayloadAndReceiveBuffer(2, payload) // 2 for update fileName
		response := &entrytaskproto.Response{}
		if err := proto.Unmarshal(buffer, response); err != nil {
			log.Fatalln("Failed to parse reply from TCP: ", err)
		}
		if response.GetStatus() == 1 {
			// success so redirect and delete old file
			relativeFilePath := "Images/" + response.GetOldFileName()
			if _, err := os.Stat(relativeFilePath); err == nil {
				// file exists so delete it
				e := os.Remove(relativeFilePath)
				if e != nil {
					log.Fatal(e)
				}
			}
			http.Redirect(w, r, "/userpage", http.StatusFound)
		} else if response.GetStatus() == 0 {
			log.Fatal("failed to update fileName")
		} else {
			log.Fatal("unexpected error")
		}
	}
}

func setupRoutes() {
	http.HandleFunc("/", login)
	http.HandleFunc("/userpage", userpage)
	http.HandleFunc("/upload", uploadImage)
	err := http.ListenAndServe(":8081", nil) // setting listening port
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func get() (connection tcpConn) {
	connectionPool.mu.Lock()
	defer connectionPool.mu.Unlock()
	log.Println("Number of connections before get: ", len(connectionPool.pool))
	connection = <-connectionPool.pool
	return connection
}

func release(connection tcpConn) {
	connectionPool.mu.Lock()
	defer connectionPool.mu.Unlock()
	connectionPool.pool <- connection
	fmt.Println("Number of connections after release: ", len(connectionPool.pool))
}

func main() {
	// Make the connection pool here
	maxIdleConnection := 1000000
	connectionPool = &TCPConnectionPool{
		pool: make(chan tcpConn, maxIdleConnection),
	}
	for i := 0; i < maxIdleConnection; i++ {
		conn, err := net.Dial(TYPE, HOST+":"+PORT)
		tcpConnection := tcpConn{
			id:         string(i),
			connection: conn,
		}
		if err != nil {
			log.Fatal("failed to open connection for connpool: ", err)
		}
		release(tcpConnection)
	}

	fs := http.FileServer(http.Dir("./Images"))
	http.Handle("/Images/", http.StripPrefix("/Images/", fs))

	http.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "userpage.gtpl")
	})

	setupRoutes()
}
