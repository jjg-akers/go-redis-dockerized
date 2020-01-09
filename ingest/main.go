package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	//"database/sql"
	//"fmt"
	"html/template"
	//"io/ioutil"
	"github.com/go-redis/redis"
	"github.com/golang/gddo/httputil/header"
	//"github.com/gomodule/redigo/redis"
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"
)

type Data struct {
	Mascot   string
	Location string
}

type RequestBody struct {
	Endpoint struct {
		Method string
		URL    string
	}
	Data interface{}
}

type RedisJSON struct {
	RequestBody
	Time time.Time `json:"time"`
}

type successResponse struct {
	Title    string
	Time     time.Time
	Endpoint string
}

// declare a global redis client
var client *redis.Client

// compile all templates and cache them
var templates = template.Must(template.ParseGlob("static/templates/*"))

func readJSON(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {

		// grab current
		timeOfReq := time.Now()

		// *** REQUEST VALIDATION ***

		// Check if the Content-Type header has the value application/json.
		if r.Header.Get("Content-Type") != "" {
			value, _ := header.ParseValueAndParams(r.Header, "Content-Type")
			if value != "application/json" {
				msg := "Content-Type header is not application/json"
				log.Println(msg)
				http.Error(w, msg, http.StatusUnsupportedMediaType)
				return
			}
		}

		// instantiate request
		var currentRequest RequestBody

		// Use http.MaxBytesReader to enforce a maximum read of 1MB from the
		// response body. A request body larger than that will  result in
		// Decode() returning a "http: request body too large" error.
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)

		defer r.Body.Close()

		// Setup  decoder
		dec := json.NewDecoder(r.Body)

		// Use disallow unknown fields to enforce structure of incoming JSON
		// All posts should contain an Endpoint and Data keys. Endpoint must contain Method and URL
		// Data will get marshalled into an interface which should allow some flexibility
		dec.DisallowUnknownFields()

		// pass to decoder
		err := dec.Decode(&currentRequest)

		if err != nil {
			//log.Fatal(err)
			// catch error types
			// use errors.As to trigger specific cases
			// most of this code is adapted from https://www.alexedwards.net/blog/how-to-properly-parse-a-json-request-body
			var syntaxError *json.SyntaxError
			var unmarshalTypeError *json.UnmarshalTypeError

			switch {
			// Catch any syntax errors in the JSON and send an error message
			case errors.As(err, &syntaxError):
				log.Println(err.Error())
				log.Println("error decoding currentRequest - syntax error")
				// offset will help locate where the error occured
				msg := fmt.Sprintf("Request body contains badly-formed JSON (at position %d)", syntaxError.Offset)
				http.Error(w, msg, http.StatusBadRequest)

			// Catch type errors
			case errors.As(err, &unmarshalTypeError):
				log.Println(err.Error())
				log.Println("error decoding currentRequest - missmatched type")
				msg := fmt.Sprintf("Request body contains an invalid value for the %q field (at position %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset)
				http.Error(w, msg, http.StatusBadRequest)

			// Catch unexpected fields
			case strings.HasPrefix(err.Error(), "json: unknown field "):
				log.Println(err.Error())
				log.Println("error decoding currentRequest - unknown field")
				fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
				msg := fmt.Sprintf("Request body contains unknown field %s", fieldName)
				http.Error(w, msg, http.StatusBadRequest)

			// Catch empty request bodies
			case errors.Is(err, io.EOF):
				log.Println(err.Error())
				log.Println("error decoding currentRequest - empty body")
				msg := "Request body must not be empty"
				http.Error(w, msg, http.StatusBadRequest)

			// Catch request body too large
			case err.Error() == "http: request body too large":
				log.Println(err.Error())
				log.Println("error decoding currentRequest - body too large")
				msg := "Request body must not be larger than 1MB"
				http.Error(w, msg, http.StatusRequestEntityTooLarge)

			// Catch everything else with a default
			// log the error and send 500 Internal Server Error
			default:
				log.Println(err.Error())
				log.Println("error decoding currentRequest")
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			return
		}
		// Now check that the request body only contained a single JSON object.
		if dec.More() {
			msg := "Request body must only contain a single JSON object"
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		// if no errors, write response and push onto redis
		// simple response
		w.WriteHeader(http.StatusAccepted)
		//fmt.Fprintf(w, "Successfully processed: %s", currentRequest.Endpoint.URL)
		//w.Header().Set(currentRequest.Endpoint.URL, "Success")

		//**** Try JSON response
		// set header type
		// w.Header().Set("Content-Type", "application/json")
		// // make response
		// postResponse := successResponse{
		// 	Title:    "Successfully Processed Request",
		// 	Time:     time.Now(),
		// 	Endpoint: currentRequest.Endpoint.URL,
		// }
		// // encode
		// json.NewEncoder(w).Encode(postResponse)

		// test the client
		// pong, err := client.Ping().Result()
		// fmt.Println(pong, err)

		//add time to data
		currentJSON := RedisJSON{
			RequestBody: currentRequest,
			Time:        timeOfReq,
		}

		//** TODO SET UP PIPELINING ***

		// push onto redis list
		// _, err = client.RPush("mylist", newPost).Result()
		// if err != nil {
		// 	log.Println("error in RPush: ")
		// 	log.Println(err)
		// 	log.Println(string(newPost))
		// 	return

		// }

		// try using as routine
		// how to test connection pool?
		//
		// this doesn't actually make sense. listen and serve is itself a goroutine, so there should
		// only be one task per routine anyways
		go func(post RedisJSON) {
			//fmt.Println("new routine")
			// After JSON decoding and validation, marshall information into byte slice
			newPost, err := json.Marshal(currentJSON)
			if err != nil {
				log.Println("error in json marshall")
				return
			}
			_, err = client.RPush("mylist", newPost).Result()
			if err != nil {
				log.Println("error in RPush: ")
				log.Println(err)
				log.Println(string(newPost))
				return

			}
			//time.Sleep(5 * time.Second)

			return
			// *** need to figure out how to manage and notify when errors occur duing push onto redis

		}(currentJSON)

		//go client.RPush("mylist", newPost).Result()

		// if request was successfully processed and passed to redis, send success response

		//*****************
		// write back to page
		//fmt.Fprintf(w, "Person: %+v", currentRequest)
		// you access the cached templates with the defined name, not the filename
		// err = templates.ExecuteTemplate(w, "ingestPage", currentRequest)
		// if err != nil {
		// 	http.Error(w, err.Error(), http.StatusInternalServerError)
		// 	return
		// }

	} else {
		// handle get request

		// testing templates
		var defaultData Data
		defaultData.Location = "none"
		defaultData.Mascot = "none"

		var defaultrequest RequestBody
		defaultrequest.Endpoint.Method = "none"
		defaultrequest.Endpoint.URL = "none"

		//err := templates.ExecuteTemplate(w, "ingestPage", nil)
		err := templates.ExecuteTemplate(w, "ingestPage", defaultrequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func RPush(client *redis.Client, key string, valueList []string) (bool, error) {
	err := client.RPush(key, valueList).Err()
	return true, err
}

func SetValue(client *redis.Client, key string, value interface{}) (bool, error) {
	serializedValue, _ := json.Marshal(value)
	err := client.Set(key, string(serializedValue), 0).Err()
	return true, err
}

func GetValue(client *redis.Client, key string) (interface{}, error) {
	var deserializedValue interface{}
	serializedValue, err := client.Get(key).Result()
	json.Unmarshal([]byte(serializedValue), &deserializedValue)
	return deserializedValue, err
}

func main() {
	fmt.Println("Application Starting")
	// setup logging file
	// give options to either create if not exist, or append if exists
	logFile, err := os.OpenFile("./logs/go-server.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Could not connect to log file")
		log.Fatal(err)
	}
	defer logFile.Close()
	// set output for loggin
	log.SetOutput(logFile)

	// setup server
	router := http.NewServeMux()
	// set route
	router.HandleFunc("/", readJSON)

	// create server
	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
		//ErrorLog:     logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	//go-redis Create Redis Client
	client = redis.NewClient(&redis.Options{
		//Addr:         "redis:6379",
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		PoolSize:     10,
		PoolTimeout:  30 * time.Second,
	})

	// try redis pool
	// var redisPool *redis.Client
	// redisPool = redis.NewClient(&redis.Options{
	// 	Addr:         ":6379",
	// 	DialTimeout:  10 * time.Second,
	// 	ReadTimeout:  30 * time.Second,
	// 	WriteTimeout: 30 * time.Second,
	// 	PoolSize:     10,
	// 	PoolTimeout:  30 * time.Second,
	// })

	//conn := client.Conn()

	//conn := redis.Conn

	//_, err = redisPool.c

	_, err = client.Ping().Result()
	if err != nil {
		log.Println("Failed ping to redis client")
		log.Fatal(err)
	}
	log.Println("Successful connection to redis")

	// make channel to notify when server has successfully shut down
	done := make(chan bool, 1)

	// make channel to reciece quit signal and initiate graceful server shutdown
	quit := make(chan os.Signal, 1)

	//Use Notify to relay incoming signals to to quit cannel
	signal.Notify(quit, os.Interrupt)

	// start a goroutine with the server, and two channels
	// quit will be used to intercept shut down signel
	// done will be used to end main routine
	go serverShutDown(server, quit, done)

	// start server
	// go server will start a new routine for every post
	// *** if multiple posts come in at same time, how will redis handle?
	// Can we use connection pool to avoid collisions? does redis block during connection?
	err = server.ListenAndServe()
	if err != nil {
		log.Println("Server failed to listen")
		log.Fatal(err)
	}

	// this will block until signal is recieved
	<-done
	log.Println("Server stopped")

	fmt.Println("Application stopped")
}

// function to shutdown server
func serverShutDown(server *http.Server, quit <-chan os.Signal, done chan<- bool) {
	// wait for signal in quit channel
	<-quit

	// once interrupt signal is recieved, start the server shutdown
	log.Println("Interrupt recieved, shutting server down")

	// give the server 30 seconds to shut down
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// tell server to not keep any existing connections alive
	server.SetKeepAlivesEnabled(false)

	err := server.Shutdown(ctx)
	if err != nil {
		log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
	}

	// close the done channel to tell main goroutine that server shutdown is complete
	close(done)
}
