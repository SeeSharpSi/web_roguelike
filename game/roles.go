package game

type Role string

const (
	KillP1Role        Role = "kill player 1"
	KillP2Role        Role = "kill player 2"
	KillP3Role        Role = "kill player 3"
	KillP4Role        Role = "kill player 4"
	KillIntruderRole  Role = "kill an intruder"
	ExploreSectorRole Role = "explore the sector"
	FindKeycardRole   Role = "find a keycard"
	SurviveRole       Role = "survive the expedition"
)

var characterRoles = []Role{
	KillP1Role,
	KillP2Role,
	KillP3Role,
	KillP4Role,
	KillIntruderRole,
	ExploreSectorRole,
	FindKeycardRole,
	SurviveRole,
}

var characterNames = []string{
	"Eli Rane",
	"Mira Voss",
	"Sana Quill",
	"Orrin Vale",
	"Niko Sable",
	"Tessa Wren",
	"Kael Dorne",
	"Lena Vey",
	"Arin Locke",
	"Iona Reed",
	"Dax Mercer",
	"Vera Flint",
	"Ren Ash",
	"Lyra Holt",
	"Silas Kest",
	"Nessa Ward",
	"Torin Hale",
	"Ada Venn",
	"Kira Thorne",
	"Ezra Marr",
}
