package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	//"database/sql"
	"github.com/go-redis/redis"
	"io/ioutil"
	//"github.com/golang/gddo/httputil/header"
	//"github.com/gomodule/redigo/redis"
	"errors"
	"os"
	"regexp"
	"time"
)

type RequestBody struct {
	Endpoint struct {
		Method string
		URL    string
	}
	Data []interface{}
	Time time.Time
}

type postbackRequest struct {
	method string
	URL    string
	time   time.Time
}

//Data map[interface{}]interface{}

// can this be called as a goroutine?
// send it a client from pool
//done chan<- bool
func redisDequeue(client *redis.Client, done chan<- bool) {
	var errorCount int
	//var endpointURLRaw string
	var endpointURL string
	var endpointMethod string

	// can be a very long for loop, break this up into its own goroutine?
	//for i := 0; i < int(queueSize); i++ {

	// make a while loop to will continue as long as redis list still has values
	// This will kick off if there is at least 1 entry in list but won't neccessaritly stop after
	//		that one is done.  If more items are added to the list while it runs, it will
	//		continue.

	// Could we move all this to a go routine?
	//	- need to figure out correct way to use redis connection pool, have program wait for
	//		next available conn

	for client.LLen("mylist").Val() > 0 {
		//fmt.Println("starting loop")
		// reinitialize requstBody object
		var postback RequestBody

		// print the list length for debugging/ monitoring
		fmt.Println(client.LLen("mylist").Val())

		// pop next entry
		dequeueResp, err := client.LPop("mylist").Result()
		if err != nil {
			log.Println("Error with LPop")
			errorCount++

		}

		// convert deque to []byte
		byteSliceDequeue := []byte(dequeueResp)

		// decode into JSON & save into an struct
		err = json.Unmarshal(byteSliceDequeue, &postback)

		//fmt.Println(postback)

		// **** build request string ****
		// get endpoint info
		endpointURL = string(postback.Endpoint.URL)
		endpointMethod = postback.Endpoint.Method
		//fmt.Println("postback endpoint method: ", endpointMethod)
		//fmt.Println("postback endpoint RAWURL: ", endpointURL)

		// pull key's out of data
		if len(postback.Data) != 0 {
			for key, value := range postback.Data[0].(map[string]interface{}) {
				// add {} and convert to string
				key = "{" + key + "}"
				key = string(key)
				// convert value interface to string
				valueAsString := fmt.Sprintf("%v", value)
				//fmt.Println("Key:", key, "\nValue:", value)
				// search endpoint string for key and replace with key's value
				endpointURL = strings.Replace(endpointURL, key, valueAsString, -1)

			}
		} else {
			log.Println("No data provided for endpoint: ", postback.Endpoint)
		}

		// remove keys that were not provided
		URLregex, err := regexp.Compile(`\{([^}]+)\}`)
		//fmt.Printf("Pattern: %v\n", URLregex.String())             // print pattern
		//fmt.Println("Matched:", URLregex.MatchString(endpointURL)) // true

		// DEBUG Print submatches
		// submatchall := URLregex.FindAllString(endpointURL, -1)
		// for _, element := range submatchall {
		// 	element = strings.Trim(element, "{")
		// 	element = strings.Trim(element, "}")
		// 	fmt.Println(element)
		// }

		// remove all the unused {} using the regex
		endpointURL = URLregex.ReplaceAllString(endpointURL, "")

		//fmt.Println("\nendpoint URL after: ", endpointURL)

		// create final postback object
		currentPostBack := postbackRequest{
			method: endpointMethod,
			URL:    endpointURL,
			time:   postback.Time,
		}

		// use go routine to send http request
		go executeRequest(currentPostBack)

	}

	// when done with this batch, send the done signal
	//fmt.Println("Erros: ", errorCount)
	done <- true
	//return errorCount
}

func executeRequest(postback postbackRequest) error {
	//fmt.Println("started executerRequest")
	// test get request
	URL := postback.URL
	if postback.method == "GET" {
		//fmt.Println("starting get")
		resp, err := http.Get(URL)
		if err != nil {
			log.Println("Error making http.Get()")
			log.Println(err)
			return err
		}

		defer resp.Body.Close()

		// Check status and log errors
		if resp.StatusCode > 299 || resp.StatusCode < 200 {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("could not read body of response")
			}
			log.Println(resp.Status)
			log.Println(string(body))

		}
		log.Println("Postback successful, time taken: ", time.Now().Sub(postback.time))
		return nil

	}
	// this is where we will implement POST methods
	err := errors.New("emit macho dwarf: elf header corrupted")
	log.Println(err)
	return nil

}

func printHeader(response http.Request) {
	for key, value := range response.Header {
		fmt.Println("header key: " + key)
		fmt.Println("value: ", value)
	}

}

func printBody(response http.Request) {
	// defer closing object
	defer response.Body.Close()

	// read response body
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatalln(err)
	}

	log.Println(string(body))
}

// *************** MAIN ****************
func main() {
	fmt.Println("Application Starting")
	// setup logging file
	// give options to either create if not exist, or append if exists
	logFile, err := os.OpenFile("./logs/postback.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Could not open log file")
		log.Fatal(err)
	}
	defer logFile.Close()
	// set output for loggin
	log.SetOutput(logFile)

	//go-redis Create Redis Client
	client := redis.NewClient(&redis.Options{
		//Addr:     "redis:6379",
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	// test connection
	_, err = client.Ping().Result()
	if err != nil {
		log.Println("error with PING: ")
		log.Fatal(err)
	} else {
		log.Println("Successfully connected to redis")
	}

	//**** use channel to gracefully stop ****
	// **** try using BRPOP *****

	done := make(chan bool, 1)
	// make an infinite loop to check for updates in DB
	for {
		// check if the list has any values in it
		listSize, err := client.LLen("mylist").Result()
		if err != nil {
			log.Println("error with LLen: ", err)
		}

		if listSize > 0 {
			// pull a redis client from pool and start a routine to pull off all the current
			// 		redis values
			// call a function to pop off the values
			redisDequeue(client, done)
			//fmt.Println("errors: ", numberOfError)

			// block here -- testing go channels
			//fmt.Println("blocking")
			<-done
			//fmt.Println("done blocking")
		}

	}

	// CLOSE CONNECTIONS
}
