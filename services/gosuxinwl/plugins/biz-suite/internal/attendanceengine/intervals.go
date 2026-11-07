package attendanceengine

import "sort"

type span struct{ start, end int64 }

func intersect(a, b span) (span, bool) {
	s := span{max(a.start, b.start), min(a.end, b.end)}
	return s, s.end > s.start
}
func union(in []span) []span {
	x := append([]span(nil), in...)
	sort.Slice(x, func(i, j int) bool {
		if x[i].start == x[j].start {
			return x[i].end < x[j].end
		}
		return x[i].start < x[j].start
	})
	out := []span{}
	for _, v := range x {
		if v.end <= v.start {
			continue
		}
		n := len(out)
		if n > 0 && v.start <= out[n-1].end {
			out[n-1].end = max(out[n-1].end, v.end)
		} else {
			out = append(out, v)
		}
	}
	return out
}
func duration(in []span) int64 {
	var n int64
	for _, v := range union(in) {
		n += v.end - v.start
	}
	return n
}
func overlap(a, b []span) []span {
	out := []span{}
	for _, x := range a {
		for _, y := range b {
			if z, ok := intersect(x, y); ok {
				out = append(out, z)
			}
		}
	}
	return union(out)
}
func subtract(a, b []span) []span {
	out := []span{}
	cuts := union(b)
	for _, item := range union(a) {
		cursor := item.start
		for _, cut := range cuts {
			if cut.end <= cursor || cut.start >= item.end {
				continue
			}
			if cut.start > cursor {
				out = append(out, span{cursor, min(cut.start, item.end)})
			}
			cursor = max(cursor, cut.end)
			if cursor >= item.end {
				break
			}
		}
		if cursor < item.end {
			out = append(out, span{cursor, item.end})
		}
	}
	return out
}
func contains(in []span, t int64) bool {
	for _, s := range in {
		if t >= s.start && t < s.end {
			return true
		}
	}
	return false
}
func absolute(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
func appendUnique(values []string, value string) []string {
	for _, s := range values {
		if s == value {
			return values
		}
	}
	return append(values, value)
}
func evidenceID(kind, id string) string { return kind + ":" + id }
