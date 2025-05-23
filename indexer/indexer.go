package indexer

import idxrcore "github.com/ipni/go-indexer-core"

type Indexer interface {
	idxrcore.Interface
	HasIndexedStore() bool
}

var _ Indexer = (*indexer)(nil)

type indexer struct {
	idxrcore.Interface
	indexedStore bool
}

func New(core idxrcore.Interface, indexedStore bool) *indexer {
	return &indexer{
		Interface:    core,
		indexedStore: indexedStore,
	}
}

func (i *indexer) HasIndexedStore() bool {
	return i.indexedStore
}
