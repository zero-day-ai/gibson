package taxonomy

import (
	"embed"
)

// taxonomyFS embeds all taxonomy YAML files at compile time.
// This includes the root taxonomy.yaml file and all node, relationship, and technique definitions.
//
// The embedded filesystem is used by the TaxonomyLoader to load the canonical taxonomy
// that ships with each Gibson binary release. The taxonomy version is tied to the Gibson
// binary version to ensure reproducible behavior.
//
//go:embed *.yaml nodes/*.yaml relationships/*.yaml techniques/*.yaml
var taxonomyFS embed.FS

// GetEmbeddedFS returns the embedded filesystem containing all taxonomy YAML files.
// This is the primary interface for accessing the bundled taxonomy definitions.
func GetEmbeddedFS() embed.FS {
	return taxonomyFS
}
