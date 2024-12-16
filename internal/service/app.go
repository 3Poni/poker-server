package app

import (
	"context"
	"encoding/json"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"log"
	"math/rand"
	"net/http"
	hub2 "poker-server/internal/transport/websocket"
	client2 "poker-server/internal/transport/websocket/client"
	"poker-server/internal/transport/websocket/responder"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Player struct {
	Id         string
	client     *client2.Client
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

type Request1 struct {
	Action  string
	Bet     int
	Message string
}

type Game struct {
	action string
}

func RunGame(h *hub2.Hub) {
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
			time.Sleep(5 * time.Second)
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

func startGame(h *hub2.Hub) {
	time.Sleep(1 * time.Second)
	r := responder.JSONResponse{
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

func sendResponse(players []Player, r responder.JSONResponse) {
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

func (g *Game) Call() {
	playerBet := active[0].currentBet
	chipsToCut := currentBet - playerBet
	active[0].currentBet = currentBet
	if chipsToCut > active[0].chips {
		chipsToCut = active[0].chips
		active[0].chips = 0
		allIn()
	} else {
		active[0].chips -= chipsToCut
	}
	active[0].bet += chipsToCut
	active[0].status = "call"
	bank += chipsToCut
	var x Player
	x, active = active[0], active[1:] //shift
	passed = append(passed, x)        // push

	g.action = "turn"
}
func (g *Game) Raise() {
	chipsToCut := active[0].raiseBet - active[0].currentBet
	active[0].currentBet = active[0].raiseBet
	active[0].chips -= chipsToCut
	active[0].bet += chipsToCut
	bank += chipsToCut
	currentBet = active[0].raiseBet
	active[0].raiseBet = 0
	active[0].status = "raise"
	active = slices.Concat(active, passed)
	passed = []Player{}

	if active[0].chips <= 0 {
		allIn()
	} else {
		var x Player
		x, active = active[0], active[1:] //shift
		passed = append(passed, x)        // push
	}

	g.action = "turn"
}
func (g *Game) Fold() {
	active[0].status = "fold"
	var x Player
	x, active = active[0], active[1:] //shift
	foldSlice = append(foldSlice, x)  // push

	g.action = "turn"
}
func (g *Game) Check() {
	active[0].status = "check"
	var p Player
	p, active = active[0], active[1:] // shift
	passed = append(passed, p)        // push
	g.action = "turn"
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

type GameDefiner struct {
	highSuit     string
	subSuit      string
	high         uint8
	score        uint8
	subscore     uint8
	cardsWinId   []int
	cardsWinSuit string
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
	result["high"] = gd.high
	result["score"] = gd.score
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

func (gd *GameDefiner) checkHigh(cardsHand []Card) {
	if cardsHand[0].Id > cardsHand[1].Id {
		gd.highSuit = cardsHand[0].Suit
	} else {
		gd.highSuit = cardsHand[1].Suit
	}
	if cardsHand[0].Id > cardsHand[1].Id {
		gd.subSuit = cardsHand[0].Suit
	} else {
		gd.subSuit = cardsHand[1].Suit
	}
	if cardsHand[0].Id > cardsHand[1].Id {
		gd.high = cardsHand[0].Id
	} else {
		gd.high = cardsHand[1].Id
	}
	if cardsHand[0].Id > cardsHand[1].Id {
		gd.subscore = cardsHand[0].Id
	} else {
		gd.subscore = cardsHand[1].Id
	}
}
func (gd *GameDefiner) checkPairs(tableScores map[uint8]int) {
	var pairs = make(map[int]int)
	var three = make(map[int]int)
	for id, count := range tableScores {
		if count == 2 {
			pairs[int(id)] = count
		}
		if count == 3 {
			three[int(id)] = count
		}
		if count == 2 && gd.score <= 2 {
			gd.cardsWinId = []int{}
			maxPairId := getMax(pairs)
			gd.cardsWinId = append(gd.cardsWinId, maxPairId) // push
			gd.score = 2                                     // PAIR
		}
		if count == 2 && gd.score <= 3 && len(pairs) > 1 {
			gd.cardsWinId = []int{}
			maxPairId := getMax(pairs)
			countToSave := pairs[maxPairId]
			delete(pairs, maxPairId)
			preMaxPairId := getMax(pairs)
			pairs[maxPairId] = countToSave
			gd.cardsWinId = append(gd.cardsWinId, maxPairId, preMaxPairId)
			gd.score = 3 // TWO PAIR
		}
		if count == 3 && gd.score <= 4 {
			gd.cardsWinId = []int{}
			maxThreeId := getMax(three)
			gd.cardsWinId = append(gd.cardsWinId, maxThreeId)
			gd.score = 4 // THREE
		}
		if len(three) == 1 && len(pairs) > 0 {
			gd.cardsWinId = []int{}
			maxPairId := getMax(pairs)
			maxThreeId := getMax(three)
			gd.cardsWinId = append(gd.cardsWinId, maxThreeId, maxPairId)
			gd.score = 7 // FULL HOUSE
		}
		if count == 4 {
			gd.cardsWinId = []int{}
			gd.cardsWinId = append(gd.cardsWinId, int(id))
			gd.score = 8 // FOUR
		}
	}
}
func (gd *GameDefiner) checkStraight(tableScores map[uint8]int) {
	prevCard := 0
	var streak = 0
	var firstCard = 0
	var keys []int
	for k, _ := range tableScores {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	if gd.wheelCase(keys, 5) {
		return
	}
	for i := 0; i < 5; i++ {
		if streak == 5 {
			gd.score = 5 // STRAIGHT
			gd.cardsWinId = []int{}
			for j := firstCard; j >= firstCard-4; j-- {
				gd.cardsWinId = append(gd.cardsWinId, j)
			}
			return
		}
		if streak == 0 {
			firstCard = keys[i]
			prevCard = keys[i]
			streak++
		} else {
			if prevCard-1 == keys[i] {
				prevCard = keys[i]
				streak++
			} else {
				streak = 0
			}
		}
	}
}
func (gd *GameDefiner) wheelCase(cardsGiven []int, scoreSet int) bool {
	wheelCaseCards := [...]int{14, 2, 3, 4, 5}
	notIn := false
	for i := 0; i < 5; i++ {
		notIn = slices.Contains(cardsGiven, wheelCaseCards[i])
		if notIn == false {
			return false
		}
	}
	gd.cardsWinId = []int{14, 2, 3, 4, 5}
	gd.score = uint8(scoreSet) // STRAIGHT
	return true
}
func (gd *GameDefiner) checkFlush(tableSuits map[string]int, cardsHand []Card, cardsTable []Card) {
	for k, s := range tableSuits {
		if s >= 5 {
			gd.score = 6 // FLUSH
			gd.cardsWinId = []int{}
			gd.cardsWinSuit = k
			var allCards = slices.Concat(cardsHand, cardsTable)
			for _, card := range allCards {
				if card.Suit == gd.cardsWinSuit && gd.subscore < card.Id {
					gd.subscore = card.Id
				}
			}
			gd.checkStraightFlush(tableSuits, cardsHand, cardsTable)
		}
	}
}
func (gd *GameDefiner) checkStraightFlush(tableSuits map[string]int, cardsHand []Card, cardsTable []Card) {
	if gd.score != 6 {
		gd.checkFlush(tableSuits, cardsHand, cardsTable)
		return
	}
	prevCard := 0
	var streak = 0
	var firstCard = 0
	var keys []int
	for _, card := range slices.Concat(cardsHand, cardsTable) {
		//if card.suit == gd.cardsWinSuit { cardsToCheck = append(cardsToCheck, card) }
		if card.Suit == gd.cardsWinSuit {
			keys = append(keys, int(card.Id))
		}
	}
	sort.Ints(keys)
	//sort.Slice(keys, func(i, j int) bool {
	//	return keys[i] > keys[j]
	//})
	if gd.wheelCase(keys, 9) {
		return
	}
	for i := 0; i < 5; i++ {
		if streak == 5 {
			gd.score = 9 // STRAIGHT FLUSH
			gd.cardsWinId = []int{}
			for j := firstCard; j >= firstCard-4; j-- {
				gd.cardsWinId = append(gd.cardsWinId, j)
			}
			i = 6
		}
		if streak == 0 {
			firstCard = keys[i]
			prevCard = keys[i]
			streak++
		} else {
			if prevCard-1 == keys[i] {
				prevCard = keys[i]
				streak++
			} else {
				streak = 0
			}
		}
	}
	if gd.score == 9 {
		gd.checkRoyal(keys)
	}
}
func (gd *GameDefiner) checkRoyal(cardsGiven []int) {
	cardsNeeded := [...]int{10, 11, 12, 13, 14}
	notIn := false
	for i := 0; i < 5; i++ {
		notIn = slices.Contains(cardsGiven, cardsNeeded[i])
		if notIn == false {
			return
		}
	}
	gd.score = 10 // ROYAL
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

func getMax(mapToFind map[int]int) int {
	maxVal := 0
	var maxKey int
	for k, v := range mapToFind {
		if v > maxVal {
			maxVal = v
			maxKey = k
		}
		if v == maxVal && k > maxKey {
			maxKey = k
		}
	}
	return maxKey
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
			r := client2.Request{}
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

func setPlayers(amount int, h *hub2.Hub) {
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
