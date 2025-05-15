package config

// Datastore tracks the configuration of the datastore.
type Datastore struct {
	// Type is the type of datastore.
	// Currently, only "levelds" and "dynamodb" are supported.
	Type string
	// Dir is the directory where the datastore is kept. If this is not an
	// absolute path then the location is relative to the indexer repo
	// directory.
	// For datastores of type "dynamodb", this is the name of the table.
	Dir string
	// Region is the region where the datastore is kept. This is only used for
	// datastores of type "dynamodb".
	Region string

	// TmpType is the type of datastore for temporary persisted data.
	// Currently, only "levelds" and "dynamodb" are supported.
	TmpType string
	// TmpDir is the directory where the datastore for persisted temporary data
	// is kept. This datastore contains temporary items such as synced
	// advertisement data and data-transfer session state. If this is not an
	// absolute path then the location is relative to the indexer repo.
	// For datastores of type "dynamodb", this is the name of the temporary items table.
	TmpDir string
	// TmpRegion is the region where the datastore is kept. This is only used for
	// datastores of type "dynamodb".
	TmpRegion string

	// RemoveTmpAtStart causes all temproary data to be removed at startup.
	// Not applicable to DynamoDB-backed datastores.
	RemoveTmpAtStart bool
}

// NewDatastore returns Datastore with values set to their defaults.
func NewDatastore() Datastore {
	return Datastore{
		Dir:     "datastore",
		Type:    "levelds",
		TmpDir:  "tmpstore",
		TmpType: "levelds",
	}
}

// populateUnset replaces zero-values in the config with default values.
func (c *Datastore) populateUnset() {
	def := NewDatastore()

	if c.Dir == "" {
		c.Dir = def.Dir
	}
	if c.Type == "" {
		c.Type = def.Type
	}
	if c.TmpDir == "" {
		c.TmpDir = def.TmpDir
	}
	if c.TmpType == "" {
		c.TmpType = def.TmpType
	}
}
