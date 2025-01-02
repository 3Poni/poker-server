package app

import (
	"context"
	"encoding/json"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"log"
	"math/rand"
	"net/http"
	"poker-server/internal/transport/websocket"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Player struct {
	Id         string
	client     *websocket.Client
	chips      uint32
	bet        uint32
	currentBet uint32
	raiseBet   uint32
	status     string
	Seat       uint8
	Cards      []Card
	winCards   []Card
	winOrder   uint8
	winSum     uint32
}

type Card struct {
	Id   uint8
	Suit string
}

var stage = uint8(0)
var maxStage = uint8(5)

var passed []Player
var active []Player
var allInSlice []Player
var foldSlice []Player
var currentBet uint32
var buttonTurn uint8 = 0
var bank uint32
var blind uint32
var smallBlind uint32
var table []Card
var deck []Card
var suits = [4]string{"spades", "heart", "clubs", "diamonds"}
var cards = [13]uint8{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
var waitTime time.Duration = 1

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func RunGame(h *websocket.Hub) {
	for {
		if len(h.Clients) >= 1 {
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				startGame(h)
				wg.Done()
			}()
			wg.Wait()
			h.Broadcast <- []byte("wait players game ")
		} else {
			time.Sleep(waitTime * time.Second)
			log.Println("wait players game ")
			h.Broadcast <- []byte("players low")
		}
	}
}

func makeDeck() {
	deck = []Card{}
	for _, v := range cards {
		for _, suit := range suits {
			deck = append(deck, Card{
				Id:   v,
				Suit: suit,
			})
		}
	}
}

func shuffleDeck(slice []Card) {
	for i := range slice {
		j := rand.Intn(i + 1)
		slice[i], slice[j] = slice[j], slice[i]
	}
}

func setBlinds(b uint32) {
	smallBlind = b / 2
	for k, _ := range active {
		if k == int(buttonTurn) {
			active[k].chips -= smallBlind
			active[k].currentBet = smallBlind
			active[k].bet = smallBlind
			active[k].status = "sblind"
			bank += smallBlind
		} else if k == int(buttonTurn+1) {
			active[k].chips = active[k].chips - b
			active[k].currentBet = b
			active[k].bet = b
			active[k].status = "blind"
			bank += b
			currentBet = b
		}
	}
}

func setPlayersCards() {
	for k, p := range active {
		var x []Card
		x, deck = deck[:2], deck[2:] // 2 карты сдаем
		active[k].Cards = append(p.Cards, x...)
	}
}

func startGame(h *websocket.Hub) {
	time.Sleep(1 * time.Second)
	r := JSONResponse{
		Body: map[string]any{},
		Hub:  h,
	}
	g := Game{
		action: "turn",
	}
	//h.broadcast <- []byte("game is running, clients: " + strconv.Itoa(len(h.clients)))
	setPlayers(2, h)
	stage = 0
	bank = 0
	table = []Card{}
	makeDeck()
	shuffleDeck(deck)
	log.Println(active, " active players at start")
	sort.Slice(active, func(i, j int) bool {
		return active[i].Seat < active[j].Seat // сортируем игроков
	})
	active = append(active[buttonTurn+1:], active[0:buttonTurn+1]...)
	setBlinds(20)
	active = append(active[len(active)-1:], active[0:2]...)
	setPlayersCards()
	log.Println(active, " active players AFTER start")
	players := slices.Concat(active, foldSlice, allInSlice, passed)
	r.Body["timer"] = active[0].Seat
	sendResponse(players, r)
	g.action = "turn"
	// TODO начинаться должно после blind, а не с него, + игроки делают check, когда по факту в игре call
	for {
		log.Println(" FOR CYCLE HAPPENED")
		r.ActionCalled = g.action
		if g.action == "turn" {
			log.Print("turn called ")
			g.action = nextTurn()
		} else if g.action == "round" {
			log.Print("round called ")
			g.action = "turn"
			nextRound()
		} else if g.action == "finish" {
			log.Print("finish called ")
			finishGame()
			players = slices.Concat(active, foldSlice, allInSlice, passed)
			r.Body["finishGame"] = true
			sendResponse(players, r)
			time.Sleep(155 * time.Second)
			return
		} else {
			log.Print(g.action + " called ")
			meth := reflect.ValueOf(&g).MethodByName(g.action)
			meth.Call(nil)
			//time.Sleep(1 * time.Second)
		}
		if len(active) > 0 {
			r.Body["timer"] = active[0].Seat
		} else {
			r.Body["timer"] = ""
		}
		players = slices.Concat(active, foldSlice, allInSlice, passed)
		sendResponse(players, r)
	}
}

func sendResponse(players []Player, r JSONResponse) {
	var pResp []PlayerResp
	for _, p := range players {
		var wcResp []CardsExp
		var pcResp []CardsExp
		for _, c := range p.winCards {
			wcResp = append(wcResp, CardsExp{
				Id:   c.Id,
				Suit: c.Suit,
			})
		}

		if r.Body["finishGame"] == true {
			for _, c := range p.Cards {
				pcResp = append(pcResp, CardsExp{
					Id:   c.Id,
					Suit: c.Suit,
				})
			}
		}
		pResp = append(pResp, PlayerResp{
			Id:         p.Id,
			Chips:      p.chips,
			Bet:        p.bet,
			Cards:      pcResp,
			CurrentBet: p.currentBet,
			Seat:       p.Seat,
			Status:     p.status,
			WinOrder:   p.winOrder,
			WinCards:   wcResp,
			WinSum:     p.winSum,
		})
	}
	var cResp []CardsExp
	for _, c := range table {
		cResp = append(cResp, CardsExp{
			Id:   c.Id,
			Suit: c.Suit,
		})
	}
	r.Players = players
	r.Body["players"] = pResp
	r.Body["currentBet"] = currentBet
	log.Println(currentBet, " current bet before respond")
	r.Body["tableCards"] = cResp
	r.Body["bank"] = bank
	r.Respond() // Отвечаем каждый раз здесь, заполняем body из функций
}

type PlayerResp struct {
	Id         string     `json:"id"`
	Chips      uint32     `json:"chips"`
	Bet        uint32     `json:"bet"`
	CurrentBet uint32     `json:"currentBet"`
	Seat       uint8      `json:"seat"`
	Status     string     `json:"status"`
	Cards      []CardsExp `json:"cards"`
	WinCards   []CardsExp `json:"winCards"`
	WinOrder   uint8      `json:"winOrder"`
	WinSum     uint32     `json:"winSum"`
}

type CardsExp struct {
	Id   uint8  `json:"id"`
	Suit string `json:"suit"`
}

func allIn() {
	var x Player
	x, active = active[0], active[1:]  //shift
	allInSlice = append(allInSlice, x) // push
}

func nextTurn() string {
	if len(active) > 0 {
		var wg sync.WaitGroup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var action string
		wg.Add(1)
		go func() {
			action = hold(ctx)
			if action == "hold" {
				hold(ctx)
			}
			wg.Done()
		}()
		wg.Wait()
		defer log.Println(" next turn HAPPENED")
		defer cancel()
		return action
		//passed = append(passed, active[0]) // push  TODO Убрать потом в fold, check и т.д.
		//active = active[1:]
	}

	if len(foldSlice) >= 3-1 { // тут было len(h.clients)
		//finishGame()
		return "finish"
	}

	if len(active) == 0 {
		if stage < maxStage {
			if len(allInSlice) >= len(passed)-1 {
				active, passed = append(active, passed...), []Player{}
				//finishGame()
				return "finish"
			}
			stage++
			if stage == 4 {
				active, passed = append(active, passed...), []Player{}
				//finishGame()
				return "finish"
			} else {
				time.Sleep(2 * time.Second)
				//nextRound()
				return "round"
			}
		} else {
			//finishGame()
			return "finish"
		}
	}
	return "turn"
}
func finishGame() {
	active = slices.Concat(active, allInSlice)
	defineWinner()
	return
	// TODO Все что ниже перенеси куда-нить
	active = slices.Concat(active, passed, allInSlice, foldSlice)
	allInSlice = []Player{}
	passed = []Player{}
	foldSlice = []Player{}
	buttonTurn += 1
	if buttonTurn == 5 {
		buttonTurn = 0
	}
	for _, p := range active {
		p.currentBet = 0
		p.bet = 0
		p.status = ""
		p.Cards = []Card{}
		p.raiseBet = 0
	}
}
func defineWinner() {
	log.Println("in win definer", active)
	var scores = make(map[string]map[string]any)
	for _, p := range active {
		scores[p.Id] = getScore(p.Cards)
	}
	givePrize(scores, 0)
	log.Println(active, " winners")
}

func givePrize(scores map[string]map[string]any, prizeOrder int) {
	var winners []string
	var score = 0
	var high = 0
	var subscore = 0
	for k, v := range scores {
		if v["score"].(int) > score {
			high = v["high"].(int)
			score = v["score"].(int)
			subscore = v["subscore"].(int)
			winners = []string{}
			winners = append(winners, k)
		} else if v["score"].(int) == score && v["subscore"].(int) > subscore {
			subscore = v["subscore"].(int)
			winners = []string{}
			winners = append(winners, k)
		} else if v["score"].(int) == score && v["subscore"].(int) == subscore {
			winners = append(winners, k)
		}
		if score == 1 && v["score"].(int) == 1 && v["high"].(int) > high {
			winners = []string{}
			winners = append(winners, k)
			scores[k]["winCards"] = append(v["winCards"].([]Card), Card{Id: uint8(high), Suit: v["highSuit"].(string)}) // Добавляем high карту, если надо
		} else if score == 1 && v["score"].(int) == 1 && v["high"].(int) == high {
			scores[k]["winCards"] = append(v["winCards"].([]Card), Card{Id: uint8(high), Suit: v["highSuit"].(string)}) // Добавляем high карту, если надо
			winners = append(winners, k)
		}
	}

	//if len(allInSlice) != 0 {
	//	sort.Slice(active, func(i, j int) bool {
	//		return active[i].bet < active[j].bet // сортируем по ставке
	//	})
	//}
	for k, p := range active {
		if slices.Contains(winners, p.Id) {
			log.Println(p.Id, " winner")
			prize := uint32(0)
			if prizeOrder == 0 {
				prize = (p.bet * uint32(len(active)+len(allInSlice))) / uint32(len(winners))
			} else {
				prize = bank / uint32(len(winners))
			}
			bank = bank - prize
			active[k].chips += prize
			active[k].status = "winner"
			active[k].winCards = scores[p.Id]["winCards"].([]Card)
			active[k].winOrder = uint8(prizeOrder)
			active[k].winSum = prize
			// TODO winCards должны показывать карты на столе и карты в руке
			if bank < 0 {
				active[k].chips += bank
				bank = 0
			} else if bank > 0 && len(allInSlice) == 0 {
				active[k].chips += bank
				bank = 0
			}
			delete(scores, p.Id) // Если игроку дали выигрыш, удаляем его
		}
	}
	if bank > 0 && len(allInSlice) > 0 {
		givePrize(scores, prizeOrder+1)
	}
}

func getScore(cardsHand []Card) map[string]any {
	var result = make(map[string]any)
	var tableScores = make(map[uint8]int)
	var tableSuits = make(map[string]int)
	gd := GameDefiner{
		highSuit:     "",
		subSuit:      "",
		high:         0,
		score:        0,
		subscore:     0,
		cardsWinSuit: "",
	}
	cardsTable := table

	sort.Slice(cardsTable, func(i, j int) bool {
		return cardsTable[i].Id < cardsTable[j].Id // сортируем карты
	})
	log.Println(cardsTable, " cards table ")
	log.Println(cardsHand, " cards hand ")

	for _, card := range cardsTable {
		// TODO разберись с этим
		tableScores[card.Id] = tableScores[card.Id] + 1
		if card.Id == 14 {
			tableScores[1] = tableScores[1] + 1
		}
		tableSuits[card.Suit] = tableSuits[card.Suit] + 1
	}

	for _, card := range cardsHand {
		// TODO разберись с этим
		tableScores[card.Id] = tableScores[card.Id] + 1
		tableSuits[card.Suit] = tableSuits[card.Suit] + 1
	}

	gd.checkHigh(cardsHand)
	log.Println(cardsHand, " checkHigh passed")
	gd.checkPairs(tableScores)
	log.Println(cardsHand, " checkPairs passed")
	if gd.score <= 4 {
		gd.checkStraight(tableScores)
		log.Println(cardsHand, " checkStraight passed")
		gd.checkFlush(tableSuits, cardsHand, cardsTable)
		log.Println(cardsHand, " checkFlush passed")
	} else if gd.score == 7 {
		gd.checkStraightFlush(tableSuits, cardsHand, cardsTable)
		log.Println(cardsHand, " sf passed")
	}
	// TODO high должен быть по комбинации, subscore это уже kicker, была ситуация где 10 9 9 проиграла 11 8 8
	if gd.score > 1 {
		result["subscore"] = int(gd.high)
		gd.high = 0
		for _, id := range gd.cardsWinId {
			if uint8(id) > gd.high {
				gd.high = uint8(id)
			}
		}
	} else {
		result["subscore"] = int(gd.subscore)
	}
	result["high"] = int(gd.high)
	result["score"] = int(gd.score)
	result["highSuit"] = gd.highSuit
	result["subSuit"] = gd.subSuit
	var winCards []Card
	var allCards = slices.Concat(cardsHand, cardsTable)
	for _, card := range allCards {
		if len(gd.cardsWinId) != 0 && gd.cardsWinSuit != "" {
			if slices.Contains(gd.cardsWinId, int(card.Id)) && card.Suit == gd.cardsWinSuit {
				winCards = append(winCards, card)
			}
		} else if len(gd.cardsWinId) != 0 && gd.cardsWinSuit == card.Suit {
			if slices.Contains(gd.cardsWinId, int(card.Id)) {
				winCards = append(winCards, card)
			}
		} else if len(gd.cardsWinId) == 0 && gd.cardsWinSuit == "" {
			if card.Suit == gd.cardsWinSuit {
				winCards = append(winCards, card)
			}
		}
	}
	result["winCards"] = winCards
	if len(winCards) == 0 {
		result["winCards"] = cardsHand
	}
	result["cardsWinId"] = gd.cardsWinId
	result["cardsWinSuit"] = gd.cardsWinSuit
	return result
}

func nextRound() {
	log.Println("next round, stage ", stage)
	active, passed = passed, []Player{}

	currentBet = 0
	for k, _ := range active {
		active[k].status = ""
		active[k].currentBet = 0
	}
	for k, _ := range foldSlice {
		active[k].status = ""
		active[k].currentBet = 0
	}
	if stage == 1 {
		tableAddCard(3)
	} else {
		tableAddCard(1)
	}
}

func tableAddCard(amount int) {
	var x []Card
	x, deck = deck[:amount], deck[amount:] // 2 карты сдаем
	table = append(table, x...)
}

func hold(ctx context.Context) string {
	for {
		if active[0].client == nil { // Бот ходит
			log.Print(active[0].Id, " botId on hold")
			time.Sleep(5 * time.Second)
			if currentBet > active[0].currentBet {
				return "Call" // TODO замени на Fold
			}
			return "Check"
		} else {
			log.Println(active[0].client.Action, " client ACTION --------")
		}
		select {
		case <-ctx.Done():
			if currentBet > active[0].currentBet {
				return "Fold"
			}
			return "Check"
		case message := <-active[0].client.Action:
			log.Println(message, " client request hold")
			r := websocket.Request{}
			err := json.Unmarshal(message, &r)
			if err != nil {
				log.Fatal("json unmarshal error:", err)
			}
			var actionsAvailable = []string{"call", "raise", "fold", "check"}
			if slices.Contains(actionsAvailable, r.Action) {
				log.Println("current action would be: ", r.Action)
				if uint32(r.Bet) > 0 && uint32(r.Bet) <= active[0].chips {
					active[0].raiseBet = uint32(r.Bet)
				}
				log.Println("hold is broadcasted")
				return cases.Title(language.English, cases.Compact).String(r.Action)
			}
			return "hold"
		}
	}
}

func setPlayers(amount int, h *websocket.Hub) {
	active = make([]Player, 0)

	i := 1
	for p, _ := range h.Clients {
		active = append(active, Player{
			Id:         p.Id,
			client:     p,
			chips:      1000,
			bet:        0,
			currentBet: 0,
			Seat:       uint8(i),
			status:     "",
		})
		i++
	}
	for i := i; i <= amount+len(h.Clients); i++ {
		active = append(active, Player{
			Id:         "55" + strconv.Itoa(i),
			chips:      1000,
			bet:        0,
			currentBet: 0,
			Seat:       uint8(i),
		})
	}
}
