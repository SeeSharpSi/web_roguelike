package game

type Ally struct {
	Strength int
	Stamina  int
	Health   int
	Type     EnemyType
}

type AllyType = string

const (
	AllyAlien EnemyType = "ally_alien"
	AllyHuman EnemyType = "ally_human"
)
