package stdlib

import "embed"

//go:embed gicel/*.gicel
var sourceFS embed.FS

func mustReadSource(name string) string {
	data, err := sourceFS.ReadFile("gicel/" + name + ".gicel")
	if err != nil {
		panic("stdlib: missing embedded source: " + name + ".gicel")
	}
	return string(data)
}
