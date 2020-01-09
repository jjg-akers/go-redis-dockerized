#!/bin/sh

#start redis
/usr/bin/redis-server &
#/usr/local/etc/redis/redis.conf
#--appendonly", "yes" ]

cd go/src/kochavaproject/ingest/ && go run main.go &
#go build main.go
#go run main.go
#go run /go/src/ingest/main.go &


cd go/src/kochavaproject/postback/ && go run main.go
#go build main.go
#go run /go/src/postback/main.go
