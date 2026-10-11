// Package metrics reads counters from Prometheus text exposition, e.g. the Stream Worker's
// fp_late_chunks_total. The scenario sums the value over all replicas before and after.
package metrics

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Sum returns the sum of all samples of metric name whose labels include every pair in
// match (nil = all series). found is false if no matching sample exists.
func Sum(exposition []byte, name string, match map[string]string) (total float64, found bool, err error) {
	sc := bufio.NewScanner(bytes.NewReader(exposition))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		series, rest, ok := splitSeries(l)
		if !ok {
			return 0, false, fmt.Errorf("line %d: malformed sample %q", line, l)
		}
		metric, labels, err := parseSeries(series)
		if err != nil {
			return 0, false, fmt.Errorf("line %d: %w", line, err)
		}
		if metric != name || !contains(labels, match) {
			continue
		}
		valStr := strings.Fields(rest)[0] // optional timestamp follows
		v, err := strconv.ParseFloat(valStr, 64)
		if err != nil || math.IsNaN(v) {
			return 0, false, fmt.Errorf("line %d: bad value %q", line, valStr)
		}
		total += v
		found = true
	}
	return total, found, sc.Err()
}

func splitSeries(l string) (series, rest string, ok bool) {
	if i := strings.IndexByte(l, '{'); i >= 0 {
		j := strings.LastIndexByte(l, '}')
		if j < i {
			return "", "", false
		}
		rest = strings.TrimSpace(l[j+1:])
		return l[:j+1], rest, rest != ""
	}
	f := strings.Fields(l)
	if len(f) < 2 {
		return "", "", false
	}
	return f[0], strings.Join(f[1:], " "), true
}

func parseSeries(s string) (string, map[string]string, error) {
	i := strings.IndexByte(s, '{')
	if i < 0 {
		return s, nil, nil
	}
	name := s[:i]
	body := strings.TrimSuffix(s[i+1:], "}")
	labels := map[string]string{}
	for body != "" {
		eq := strings.IndexByte(body, '=')
		if eq < 0 || len(body) < eq+2 || body[eq+1] != '"' {
			return "", nil, fmt.Errorf("bad labels in %q", s)
		}
		key := strings.TrimSpace(body[:eq])
		rest := body[eq+2:]
		var val strings.Builder
		k := 0
		for ; k < len(rest); k++ {
			if rest[k] == '\\' && k+1 < len(rest) {
				k++
				switch rest[k] {
				case 'n':
					val.WriteByte('\n')
				default:
					val.WriteByte(rest[k])
				}
				continue
			}
			if rest[k] == '"' {
				break
			}
			val.WriteByte(rest[k])
		}
		if k >= len(rest) {
			return "", nil, fmt.Errorf("unterminated label value in %q", s)
		}
		labels[key] = val.String()
		body = strings.TrimPrefix(strings.TrimSpace(rest[k+1:]), ",")
		body = strings.TrimSpace(body)
	}
	return name, labels, nil
}

func contains(labels, match map[string]string) bool {
	for k, v := range match {
		if labels[k] != v {
			return false
		}
	}
	return true
}
