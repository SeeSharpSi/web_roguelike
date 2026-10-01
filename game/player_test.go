package game

import (
	"math/rand/v2"
	"testing"
)

func TestGeneratePlayerReproducesWholePlayerFromSeed(t *testing.T) {
	first := Player{}
	first.Generate_player(rand.New(rand.NewPCG(31, 47)))

	second := Player{}
	second.Generate_player(rand.New(rand.NewPCG(31, 47)))

	if first != second {
		t.Fatalf("same seeded RNG generated different players: first=%+v second=%+v", first, second)
	}
}

func TestGeneratePlayerProducesValidVariedCharacters(t *testing.T) {
	const generations = 2048
	rng := rand.New(rand.NewPCG(101, 203))
	names := make(map[string]bool)
	roles := make(map[Role]bool)
	health := make(map[int]bool)
	stamina := make(map[int]bool)

	for index := range generations {
		player := Player{}
		player.Generate_player(rng)

		nameIsValid := false
		for _, name := range characterNames {
			if player.Name == name {
				nameIsValid = true
				break
			}
		}
		if !nameIsValid {
			t.Fatalf("generation %d produced unknown character name %q", index, player.Name)
		}

		roleIsValid := false
		for _, role := range characterRoles {
			if player.Role == role {
				roleIsValid = true
				break
			}
		}
		if !roleIsValid || player.Role == "" {
			t.Fatalf("generation %d produced invalid directive %q", index, player.Role)
		}
		if !player.Alive {
			t.Fatalf("generation %d produced inactive player %+v", index, player)
		}
		if player.Health < 83 || player.Health > 100 {
			t.Fatalf("generation %d health = %d, want 83..100", index, player.Health)
		}
		if player.Stamina < 50 || player.Stamina > 100 {
			t.Fatalf("generation %d stamina = %d, want 50..100", index, player.Stamina)
		}
		if player.Strength < 0.80 || player.Strength > 1.00 {
			t.Fatalf("generation %d strength = %.2f, want 0.80..1.00", index, player.Strength)
		}

		names[player.Name] = true
		roles[player.Role] = true
		health[player.Health] = true
		stamina[player.Stamina] = true
	}

	if len(names) < 2 {
		t.Fatalf("generated names did not vary: %v", names)
	}
	if len(roles) < 2 {
		t.Fatalf("generated directives did not vary: %v", roles)
	}
	for _, role := range []Role{KillIntruderRole, ExploreSectorRole, FindKeycardRole, SurviveRole} {
		if !roles[role] {
			t.Errorf("generated directives did not include %q", role)
		}
	}
	if len(health) < 2 {
		t.Fatalf("generated health values did not vary: %v", health)
	}
	if len(stamina) < 2 {
		t.Fatalf("generated stamina values did not vary: %v", stamina)
	}
}
