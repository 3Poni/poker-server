package responder

import (
	"encoding/json"
	"log"
	app "poker-server/internal/service"
	hub2 "poker-server/internal/transport/websocket"
)

type Responder interface {
	Respond()
}

type JSONResponse struct {
	Body         map[string]any
	Hub          *hub2.Hub
	ActionCalled string
	Players      []app.Player `json:"players"`
	Bank         int          `json:"bank"`
	TableCards   []app.Card   `json:"tableCards"`
}

func (j *JSONResponse) Respond() {
	for client, _ := range j.Hub.Clients {
		log.Println(j.Players, " j players  =======")
		for _, p := range j.Players {
			if p.Id == client.Id {
				var cArr []app.CardsExp
				for _, c := range p.Cards {
					cArr = append(cArr, app.CardsExp{
						Id:   c.Id,
						Suit: c.Suit,
					})
				}
				j.Body["hand"] = cArr
				j.Body["viewerSeat"] = p.Seat
			}
		}
		log.Println(j.Body, " print j body")
		resp, err := json.Marshal(j.Body)
		if err != nil {
			log.Println(err)
		}
		client.Send <- resp
		delete(j.Body, "hand")
	}
	// TODO нужно отправить clients(players), currentPlayerTimerId, bank, playersBets, yourTurn: true/false, cardsByPlayerId, tableCards
}
