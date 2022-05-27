# Justin's Entry Task 

Tech Stack:
MySQL + Redis -> Go -> HTML

### <b>How to run</b>
1) Run redis(port 6379)
```
docker run -d --name redis-stack-server -p 6379:6379 redis/redis-stack-server:latest
```
2) Run MySQL(port 8081)
>account: root
> 
>password: secret
> 
>database name: db
3) Import the schema into MySQL
4) Change directory into protobuf_files and run
```
protoc -I=./ --go_out=./ req.proto queries.proto replies.proto
```
2) Run TCP Server, with(y) or without(n) cache
```
go run app/tcp/* -- [y/n]
```
3) Run HTTP Server
```
go run app/http/*
```
4) Go to http://127.0.0.1:8081/ 

### <b>How to stress test</b>
1) Change directory into stess test
2) Run
```
go run fillwithdummydata.go
```
3) Install wrk and run
```
wrk -c150 -t2 -d5s -s stresstest.lua http://127.0.0.1:8081/
```

Program is capable of sustaining 4000 requests without crashing

### <b>Functionality:</b>

<i>Version 1</i>
- Login and authentication
- Updates to the MySQL DB
- View the latest information from the MySQL DB
- Usage of JWT tokens for session management


<i>Version 2</i>
- Separate into HTTP and TCP
- Implement usage of sockets
- Implement the use of protobuf
- Implement cache-aside and write-through cache using Redis

<i>Version 2</i>
- Implement connection pool
- Further optimisations


