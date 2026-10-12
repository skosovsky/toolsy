package toolsy

import "reflect"

const (
	cloneMemoSlots   = 512
	cloneMemoEntries = 384
	cloneMemoIdle    = 4
)

type cloneMemoEntry struct {
	key   cloneReference
	value reflect.Value
}

// Fixed storage is the only scratch retained between operations. Overflow maps
// are dropped on release, so graph size cannot enlarge the idle store.
type cloneMemo struct {
	entries  [cloneMemoSlots]cloneMemoEntry
	used     [cloneMemoEntries]uint16
	count    int
	overflow map[cloneReference]reflect.Value
}

//nolint:gochecknoglobals // Process-wide idle scratch has a fixed four-instance bound.
var idleCloneMemos = make(chan *cloneMemo, cloneMemoIdle)

func (c *valueCloner) lookup(key cloneReference) (reflect.Value, bool) {
	for _, entry := range c.inline[:c.inlineCount] {
		if entry.key == key {
			return entry.value, true
		}
	}
	if c.memo == nil {
		return reflect.Value{}, false
	}
	slot := cloneMemoSlot(key)
	for {
		entry := &c.memo.entries[slot]
		if entry.key.typeOf == nil {
			break
		}
		if entry.key == key {
			return entry.value, true
		}
		slot = (slot + 1) & (cloneMemoSlots - 1)
	}
	out, ok := c.memo.overflow[key]
	return out, ok
}

//nolint:mnd // Fixed integer hash mixing constants.
func cloneMemoSlot(key cloneReference) uintptr {
	// Mix alignment bits and slice length; type is checked by full key equality.
	hash := key.pointer ^ uintptr(key.length)
	hash ^= hash >> 16
	hash *= 0x45d9f3b
	hash ^= hash >> 16
	return hash & (cloneMemoSlots - 1)
}

func (c *valueCloner) remember(key cloneReference, value reflect.Value) {
	if c.memo == nil {
		if c.inlineCount < len(c.inline) {
			c.inline[c.inlineCount] = cloneMemoEntry{key: key, value: value}
			c.inlineCount++
			return
		}
		select {
		case c.memo = <-idleCloneMemos:
		default:
			c.memo = &cloneMemo{} //nolint:exhaustruct_v5 // Scratch starts with every entry empty.
		}
		for _, entry := range c.inline[:c.inlineCount] {
			c.remember(entry.key, entry.value)
		}
		clear(c.inline[:c.inlineCount])
		c.inlineCount = 0
	}
	memo := c.memo
	if memo.count == cloneMemoEntries {
		if memo.overflow == nil {
			memo.overflow = make(map[cloneReference]reflect.Value)
		}
		memo.overflow[key] = value
		return
	}
	slot := cloneMemoSlot(key)
	for memo.entries[slot].key.typeOf != nil {
		slot = (slot + 1) & (cloneMemoSlots - 1)
	}
	memo.entries[slot] = cloneMemoEntry{key: key, value: value}
	memo.used[memo.count] = uint16(slot)
	memo.count++
}

func (c *valueCloner) release() {
	clear(c.inline[:c.inlineCount])
	c.inlineCount = 0
	if c.memo == nil {
		return
	}
	memo := c.memo
	for i, slot := range memo.used[:memo.count] {
		clear(memo.entries[slot : slot+1])
		memo.used[i] = 0
	}
	memo.count = 0
	memo.overflow = nil
	c.memo = nil
	select {
	case idleCloneMemos <- memo:
	default:
	}
}
