package session

// Link fills GroupID and ParentTitle for every session.
//
// A rewind creates a new session whose parent is the session it was rewound
// from; repeated rewinds form a chain (or a tree when one session is rewound
// twice). All sessions in such a family share GroupID = id of the original
// conversation. If the original was deleted, its id is still used so the
// remaining versions stay together.
func Link(all []Session) {
	byID := make(map[string]int, len(all))
	for i := range all {
		if id := all[i].SessionID; id != "" {
			byID[id] = i
		}
	}
	for i := range all {
		s := &all[i]
		s.GroupID = rewindRoot(all, byID, i)
		if s.ParentID != "" {
			if p, ok := byID[s.ParentID]; ok {
				s.ParentTitle = all[p].Title
			}
		}
	}
}

func rewindRoot(all []Session, byID map[string]int, i int) string {
	cur := i
	for steps := 0; steps <= len(all); steps++ {
		s := &all[cur]
		if !s.IsRewind() {
			return s.SessionID
		}
		p, ok := byID[s.ParentID]
		if !ok {
			return s.ParentID
		}
		cur = p
	}
	return all[i].SessionID // cycle in parent links
}

// Collapse turns candidates (sorted newest first) into visible rows and the
// sessions nested under each row, keyed by the row's SessionID.
//
//   - collapseRewinds: each rewind family shows only its newest candidate.
//   - hideSubagents: a subagent session is nested under the row of its parent.
//
// A session is only nested when its owner is itself a candidate, so a search
// hit is never hidden without a visible row to find it under.
func Collapse(cands []Session, collapseRewinds, hideSubagents bool) ([]Session, map[string][]Session) {
	byID := make(map[string]int, len(cands))
	for i := range cands {
		if id := cands[i].SessionID; id != "" {
			if _, dup := byID[id]; !dup {
				byID[id] = i
			}
		}
	}
	newest := make(map[string]int) // GroupID -> first (newest) candidate
	if collapseRewinds {
		for i := range cands {
			if g := cands[i].GroupID; g != "" {
				if _, ok := newest[g]; !ok {
					newest[g] = i
				}
			}
		}
	}

	var ownerOf func(i, depth int) int
	ownerOf = func(i, depth int) int {
		if depth > 32 {
			return i
		}
		s := &cands[i]
		if hideSubagents && s.IsSubagent() {
			if p, ok := byID[s.ParentID]; ok && p != i {
				return ownerOf(p, depth+1)
			}
		}
		if collapseRewinds && s.GroupID != "" {
			if r := newest[s.GroupID]; r != i {
				return ownerOf(r, depth+1)
			}
		}
		return i
	}

	owner := make([]int, len(cands))
	for i := range cands {
		owner[i] = ownerOf(i, 0)
	}

	rows := make([]Session, 0, len(cands))
	rowIdx := make(map[int]int) // cand index -> rows index
	for i := range cands {
		o := owner[i]
		// Keep as a row if it owns itself, or if its owner is not a row
		// (only possible with malformed parent cycles).
		if o == i || owner[o] != o || cands[o].SessionID == "" {
			rowIdx[i] = len(rows)
			rows = append(rows, cands[i])
		}
	}

	children := make(map[string][]Session)
	for i := range cands {
		if _, isRow := rowIdx[i]; isRow {
			continue
		}
		r := &rows[rowIdx[owner[i]]]
		c := cands[i]
		if IsVersionOf(&c, r) {
			r.Versions++
		} else {
			r.Subagents++
		}
		children[r.SessionID] = append(children[r.SessionID], c)
	}
	return rows, children
}

// IsVersionOf reports whether child is an older rewind version of row.
func IsVersionOf(child, row *Session) bool {
	return child.GroupID != "" && child.GroupID == row.GroupID
}
