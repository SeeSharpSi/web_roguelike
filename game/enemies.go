package game

type Enemy struct {
	Strength float64
	Stamina  int
	Health   int
	Type     EnemyType
	Position Pos
}

type EnemyType = string

const (
	EnemyAlien EnemyType = "enemy_alien"
	EnemyHuman EnemyType = "enemy_human"
)
