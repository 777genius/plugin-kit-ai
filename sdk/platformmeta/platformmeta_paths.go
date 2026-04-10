package platformmeta

import "path"

const (
	CanonicalAuthoredRoot = "plugin"
	LegacyAuthoredRoot    = "src"
)

func authoredPath(rel string) string {
	return path.Join(CanonicalAuthoredRoot, rel)
}
