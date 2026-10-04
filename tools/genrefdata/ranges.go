package main

import "termdevtools/refdata"

// rangesFromPresence turns "seen at these versions" into version intervals.
// versions are the points observed, ascending (release branches of a
// specification, or real clusters queried); seen says whether the item was
// there at each. Every run of consecutive points where it was seen becomes
// one interval.
//
// Bounds only state what was observed. Seen from the oldest point on: no
// lower bound (the data isn't meant for anything older). Otherwise the
// interval starts at the first point where it was seen. It ends right after
// the last one — at the next minor, or at the next major when the following
// point belongs to another major.
func rangesFromPresence(versions []refdata.Version, seen []bool) []refdata.Range {
	var ranges []refdata.Range
	for i := 0; i < len(seen); i++ {
		if !seen[i] {
			continue
		}
		j := i
		for j+1 < len(seen) && seen[j+1] {
			j++
		}
		var r refdata.Range
		if i > 0 {
			min := refdata.Version{Major: versions[i].Major, Minor: versions[i].Minor}
			r.Min = &min
		}
		if j < len(seen)-1 {
			max := refdata.Version{Major: versions[j].Major, Minor: versions[j].Minor + 1}
			if versions[j+1].Major != versions[j].Major {
				max = refdata.Version{Major: versions[j+1].Major}
			}
			r.Max = &max
		}
		ranges = append(ranges, r)
		i = j
	}
	return ranges
}

// unbounded reports whether ranges is a single interval open on both sides:
// present everywhere for its distribution.
func unbounded(ranges []refdata.Range) bool {
	return len(ranges) == 1 && ranges[0] == (refdata.Range{})
}
