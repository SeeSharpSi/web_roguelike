package game

import "math/rand/v2"

type ItemType = string

const (
	ItemMedGel  ItemType = "med_gel"
	ItemCharge  ItemType = "charge"
	ItemKeycard ItemType = "keycard"
	// ItemClapper is a throwable that makes noise in the room it is thrown into.
	ItemClapper ItemType = "clapper"
)

type Item struct {
	Type ItemType
}

type itemSpawnWeight struct {
	Type   ItemType
	Weight int
}

// Larger weights make outcomes more likely; empty Type means no item, and weights <= 0 disable outcomes.
// Default weights (empty 4, each item 1) yield 50% empty rooms and 12.5% for each item.
var itemSpawnWeights = []itemSpawnWeight{
	{Weight: 4},
	{Type: ItemMedGel, Weight: 1},
	{Type: ItemCharge, Weight: 1},
	{Type: ItemKeycard, Weight: 1},
	{Type: ItemClapper, Weight: 1},
}

// randomItems selects at most one room item using the configured spawn weights.
func randomItems(rng *rand.Rand) []Item {
	totalWeight := 0
	for _, outcome := range itemSpawnWeights {
		if outcome.Weight > 0 {
			totalWeight += outcome.Weight
		}
	}
	if totalWeight == 0 {
		return nil
	}

	roll := rng.IntN(totalWeight)
	for _, outcome := range itemSpawnWeights {
		if outcome.Weight <= 0 {
			continue
		}
		if roll < outcome.Weight {
			if outcome.Type == "" {
				return nil
			}
			return []Item{{Type: outcome.Type}}
		}
		roll -= outcome.Weight
	}
	return nil
}
