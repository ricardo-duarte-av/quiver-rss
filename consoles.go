package main

import "strings"

// consoleNames maps Quiver's console ids to display names (taken from the site's frontend).
var consoleNames = map[string]string{
	"32x":       "Sega 32X",
	"3do":       "3DO Interactive Multiplayer",
	"3ds":       "Nintendo 3DS",
	"ami":       "Commodore Amiga",
	"arcade":    "Arcade",
	"cdi":       "Philips CD-i",
	"dreamcast": "Sega Dreamcast",
	"gb":        "Game Boy",
	"gba":       "Game Boy Advance",
	"gbc":       "Game Boy Color",
	"gcn":       "GameCube",
	"gen":       "Sega Genesis / Mega Drive",
	"gg":        "Sega Game Gear",
	"mobile":    "Mobile",
	"n64":       "Nintendo 64",
	"nds":       "Nintendo DS",
	"nes":       "Nintendo Entertainment System",
	"og xbox":   "Xbox",
	"pc":        "PC",
	"ps1":       "PlayStation",
	"ps2":       "PlayStation 2",
	"ps3":       "PlayStation 3",
	"ps4":       "PlayStation 4",
	"ps5":       "PlayStation 5",
	"psp":       "PlayStation Portable",
	"saturn":    "Sega Saturn",
	"segacd":    "Sega CD / Mega-CD",
	"sms":       "Sega Master System",
	"snes":      "Super Nintendo Entertainment System",
	"switch":    "Nintendo Switch",
	"switch2":   "Nintendo Switch 2",
	"vb":        "Virtual Boy",
	"vita":      "PlayStation Vita",
	"wii":       "Wii",
	"wiiu":      "Wii U",
	"x360":      "Xbox 360",
	"xone":      "Xbox One",
}

func consoleName(id string) string {
	if n, ok := consoleNames[strings.ToLower(id)]; ok {
		return n
	}
	return strings.ToUpper(id)
}

func systemsOf(ids []string) string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, consoleName(id))
	}
	return strings.Join(names, ", ")
}
