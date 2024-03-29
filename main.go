package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

var (
	port = flag.String("p", "3000", "The port to listen for HTTP traffic on")
	dir  = flag.String("d", "./", "The directory where to serve the files from")
)

func logger(f http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "NerdyGeek")
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading request body: %s", err.Error())
		}
		log.Printf("%s", payload)
		log.Printf("%s%s", r.Host, r.URL.Path)
		f.ServeHTTP(w, r)
	}
}

func main() {
	flag.Parse()
	httpMux := http.NewServeMux()
	httpMux.Handle("/", logger(http.FileServer(http.Dir(*dir))))
	fmt.Printf("Up and running on port %s \n", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", *port), httpMux))
}
