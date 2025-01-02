package main

import (
	"flag"
	"log"
	"net/http"

	app "poker-server/internal/service"
	"poker-server/internal/transport/websocket"

	"runtime/debug"
)

var addr = flag.String("addr", ":8080", "http service address")

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Println(string(debug.Stack()))
		}
	}()
	flag.Parse()
	hubConnector := websocket.NewHub()
	go app.RunGame(hubConnector)
	go hubConnector.Run()
	log.Println("Listening on", *addr)
	//http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		websocket.ServeWs(hubConnector, w, r)
	})
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
