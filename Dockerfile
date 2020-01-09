# SETUP BASE UBUNTU IMAGE
FROM ubuntu

RUN apt-get update && apt-get install -y curl 

# INSTALL REDIS
RUN apt-get install -y redis-server
RUN rm -rf /var/lib/apt/lists/*

#INSTALL GOLANG
ENV GOLANG_VERSION 1.13.5

RUN curl -sSL https://storage.googleapis.com/golang/go$GOLANG_VERSION.linux-amd64.tar.gz \
		| tar -v -C /usr/local -xz

ENV PATH /usr/local/go/bin:$PATH

RUN mkdir -p /go/src /go/bin && chmod -R 777 /go

ENV GOROOT /usr/local/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH
#WORKDIR /go

#SETUP DIRECTORIES FOR INGEST AND POSTBACK APPLICATIONS
RUN mkdir /go/src/kochavaproject
COPY . /go/src/kochavaproject

WORKDIR /go/src/kochavaproject/ingest
RUN go mod init github.com/my/repo && go get github.com/go-redis/redis/v7 && \
	go get "github.com/golang/gddo/httputil/header"

RUN go build -o main .

#COPY postback/main.go /go/src/postback
WORKDIR /go/src/kochavaproject/postback
RUN go mod init github.com/my/repo && \
	go get github.com/go-redis/redis/v7
RUN go build -o main .


# set up bash start up script
WORKDIR /
COPY start_script.sh start_script.sh

EXPOSE 6379 8080 8081

#ENTRYPOINT  ["/usr/bin/redis-server"]
RUN ["chmod", "+x", "./start_script.sh"]
CMD ["./start_script.sh"]