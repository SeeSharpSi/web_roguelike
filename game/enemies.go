package game

type Enemy struct {
	Strength int
	Stamina  int
	Health   int
	Type     EnemyType
}

type EnemyType = string

const (
	EnemyAlien EnemyType = "enemy_alien"
	EnemyHuman EnemyType = "enemy_human"
)
