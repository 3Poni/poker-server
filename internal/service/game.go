package app

import "slices"

type Game struct {
	action string
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
