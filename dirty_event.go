package trixorm

const dirtyReferencesTag = "dirtyReferences"

// DirtyEntityReferences contains single-reference IDs captured when a change is flushed.
// Entities opt in with orm:"dirtyReferences" on the embedded ORM field.
// Missing metadata means the event was produced without reference capture.
type DirtyEntityReferences struct {
	Before map[string]uint64
	After  map[string]uint64
}

func EventDirtyEntityReferences(event Event) *DirtyEntityReferences {
	data := dirtyEvent{}
	event.Unserialize(&data)
	return data.References
}

type DirtyEntityEvent interface {
	ID() uint64
	TableSchema() TableSchema
	Added() bool
	Updated() bool
	Deleted() bool
}

type dirtyEvent struct {
	I          uint64
	A          string
	E          string
	References *DirtyEntityReferences `json:",omitempty"`
}

func EventDirtyEntity(e Event) DirtyEntityEvent {
	data := dirtyEvent{}
	e.Unserialize(&data)
	schema := e.(*event).consumer.redis.engine.registry.GetTableSchema(data.E)
	return &dirtyEntityEvent{id: data.I, schema: schema, added: data.A == "i", updated: data.A == "u", deleted: data.A == "d"}
}

type dirtyEntityEvent struct {
	id      uint64
	added   bool
	updated bool
	deleted bool
	schema  TableSchema
}

func (d *dirtyEntityEvent) ID() uint64 {
	return d.id
}

func (d *dirtyEntityEvent) TableSchema() TableSchema {
	return d.schema
}

func (d *dirtyEntityEvent) Added() bool {
	return d.added
}

func (d *dirtyEntityEvent) Updated() bool {
	return d.updated
}

func (d *dirtyEntityEvent) Deleted() bool {
	return d.deleted
}
