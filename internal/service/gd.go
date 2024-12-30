package app

import (
	"slices"
	"sort"
)

type GameDefiner struct {
	highSuit     string
	subSuit      string
	high         uint8
	score        uint8
	subscore     uint8
	cardsWinId   []int
	cardsWinSuit string
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
