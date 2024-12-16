package main

import (
	"flag"
	"log"
	"net/http"
	app "poker-server/internal/service"
	hub "poker-server/internal/transport/websocket"
	clientWebsocket "poker-server/internal/transport/websocket/client"
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
	hubConnector := hub.NewHub()
	go app.RunGame(hubConnector)
	go hubConnector.Run()
	log.Println("Listening on", *addr)
	//http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		clientWebsocket.ServeWs(hubConnector, w, r)
	})
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
