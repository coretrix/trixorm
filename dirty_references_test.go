package trixorm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dirtyReferencesParent struct {
	ORM
	ID uint64
}

type dirtyReferencesChild struct {
	ORM        `orm:"redisCache;dirty=reference_changes;dirtyReferences"`
	ID         uint64
	ParentID   *dirtyReferencesParent
	OptionalID *dirtyReferencesParent
	Note       string
}

func TestDirtyReferencesCaptureInsertUpdateAndDelete(t *testing.T) {
	for _, lazy := range []bool{false, true} {
		name := "immediate"
		if lazy {
			name = "lazy"
		}
		t.Run(name, func(t *testing.T) {
			registry := &Registry{}
			registry.RegisterRedisStream("reference_changes", "default_queue", []string{"reference-group"})
			engine, cleanup := prepareTables(t, registry, 5, "", "2.0", &dirtyReferencesParent{}, &dirtyReferencesChild{})
			defer cleanup()
			first := &dirtyReferencesParent{}
			second := &dirtyReferencesParent{}
			engine.FlushMany(first, second)
			consumer := engine.GetEventBroker().Consumer("reference-group")
			consumer.DisableLoop()
			consumer.(*eventsConsumer).blockTime = time.Millisecond
			background := NewBackgroundConsumer(engine)
			background.DisableLoop()
			background.blockTime = time.Millisecond
			flush := func(child *dirtyReferencesChild) {
				if lazy {
					engine.FlushLazy(child)
					background.Digest(context.Background())
				} else {
					engine.Flush(child)
				}
			}
			check := func(before, after map[string]uint64) {
				called := false
				consumer.Consume(context.Background(), 1, func(events []Event) {
					called = true
					require.Len(t, events, 1)
					references := EventDirtyEntityReferences(events[0])
					require.NotNil(t, references)
					require.Equal(t, before, references.Before)
					require.Equal(t, after, references.After)
				})
				require.True(t, called)
			}
			child := &dirtyReferencesChild{ParentID: first}
			flush(child)
			check(nil, map[string]uint64{"ParentID": first.ID, "OptionalID": 0})
			if lazy {
				require.True(t, engine.LoadByID(1, child))
			}
			child.ParentID = second
			child.OptionalID = first
			flush(child)
			check(map[string]uint64{"ParentID": first.ID, "OptionalID": 0},
				map[string]uint64{"ParentID": second.ID, "OptionalID": first.ID})
			child.Note = "unchanged references"
			flush(child)
			check(map[string]uint64{"ParentID": second.ID, "OptionalID": first.ID},
				map[string]uint64{"ParentID": second.ID, "OptionalID": first.ID})
			child.OptionalID = nil
			flush(child)
			check(map[string]uint64{"ParentID": second.ID, "OptionalID": first.ID},
				map[string]uint64{"ParentID": second.ID, "OptionalID": 0})
			flusher := engine.NewFlusher()
			flusher.Delete(child)
			if lazy {
				flusher.FlushLazy()
				background.Digest(context.Background())
			} else {
				flusher.Flush()
			}
			check(map[string]uint64{"ParentID": second.ID, "OptionalID": 0}, nil)
			require.False(t, engine.LoadByID(child.ID, &dirtyReferencesChild{}))
			engine.MarkDirty(&dirtyReferencesChild{}, "reference_changes", child.ID)
			called := false
			consumer.Consume(context.Background(), 1, func(events []Event) {
				called = true
				require.Nil(t, EventDirtyEntityReferences(events[0]))
				require.Equal(t, child.ID, EventDirtyEntity(events[0]).ID())
			})
			require.True(t, called)
		})
	}
}
