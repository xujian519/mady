package ocr

import (
	"sort"
)

func mergeLineResults(results []Result) []Result {
	if len(results) < 2 {
		return results
	}

	type item struct {
		r    Result
		used bool
	}
	items := make([]item, len(results))
	for i, r := range results {
		items[i] = item{r: r}
	}

	type lineGroup struct {
		results []item
		yCenter int
		height  int
	}

	var groups []lineGroup
	for i := range items {
		if items[i].used {
			continue
		}
		items[i].used = true
		group := lineGroup{
			results: []item{items[i]},
			yCenter: (items[i].r.Box[1] + items[i].r.Box[3]) / 2,
			height:  items[i].r.Box[3] - items[i].r.Box[1],
		}
		for j := i + 1; j < len(items); j++ {
			if items[j].used {
				continue
			}
			yj := (items[j].r.Box[1] + items[j].r.Box[3]) / 2
			hj := items[j].r.Box[3] - items[j].r.Box[1]
			maxH := group.height
			if hj > maxH {
				maxH = hj
			}
			if abs(yj-group.yCenter) <= maxH*3/4 {
				items[j].used = true
				group.results = append(group.results, items[j])
				if yj < group.yCenter {
					group.yCenter = yj
				}
				if hj > group.height {
					group.height = hj
				}
			}
		}
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].yCenter < groups[j].yCenter
	})

	merged := make([]Result, 0, len(results))
	for _, g := range groups {
		sort.Slice(g.results, func(i, j int) bool {
			return g.results[i].r.Box[0] < g.results[j].r.Box[0]
		})
		for _, it := range g.results {
			merged = append(merged, it.r)
		}
	}
	return merged
}
