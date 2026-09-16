package game

type Ally struct {
	Strength float64
	Stamina  int
	Health   int
	Type     EnemyType
	Position Pos
}

type AllyType = string

const (
	AllyAlien EnemyType = "ally_alien"
	AllyHuman EnemyType = "ally_human"
)
