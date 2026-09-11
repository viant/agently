package preview

func (p *Package) unusedColumnWarnings() []string {
	used := map[string]map[string]bool{}
	all := map[string]bool{}
	for _, ds := range p.Datasets {
		if used[ds.DataSource] == nil {
			used[ds.DataSource] = map[string]bool{}
		}
		fields := used[ds.DataSource]
		q := ds.Query
		if q.Projection == nil {
			all[ds.DataSource] = true
		} else {
			for _, f := range append(append(append([]Field{}, q.Projection.Fields...), q.Projection.Dimensions...), q.Projection.Measures...) {
				fields[f.Field] = true
			}
		}
		var visit func(*Predicate)
		visit = func(p *Predicate) {
			if p == nil {
				return
			}
			fields[p.Field] = true
			for i := range p.And {
				visit(&p.And[i])
			}
			for i := range p.Or {
				visit(&p.Or[i])
			}
			visit(p.Not)
		}
		visit(q.Filter)
		for _, o := range q.OrderBy {
			fields[o.Field] = true
		}
		for _, b := range p.Sources[ds.DataSource].ParameterBindings {
			fields[b.Field] = true
		}
	}
	warnings := []string{}
	for _, id := range sortedKeys(p.fixtures) {
		if all[id] {
			continue
		}
		for _, c := range p.fixtures[id].columns {
			field := str(c["name"])
			if !used[id][field] {
				warnings = append(warnings, "unused column: "+id+"."+field)
			}
		}
	}
	return warnings
}
