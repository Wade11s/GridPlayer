package grid

// Template is a fixed grid with one or more big cells. Rows and columns
// count the whole grid; Big holds row, column, rowspan and colspan for
// each big cell.
type Template struct {
	Name string
	Rows int
	Cols int
	Big  [][4]int
}

// Templates are the frozen source's built-in fixed grids.
var Templates = []Template{
	{Name: "1 + 2", Rows: 2, Cols: 3, Big: [][4]int{{0, 0, 2, 2}}},
	{Name: "1 + 3", Rows: 3, Cols: 4, Big: [][4]int{{0, 0, 3, 3}}},
	{Name: "1 + 5", Rows: 3, Cols: 3, Big: [][4]int{{0, 0, 2, 2}}},
	{Name: "1 + 7", Rows: 4, Cols: 4, Big: [][4]int{{0, 0, 3, 3}}},
	{Name: "1 + 12", Rows: 4, Cols: 4, Big: [][4]int{{1, 1, 2, 2}}},
	{Name: "2 + 8", Rows: 4, Cols: 4, Big: [][4]int{{0, 0, 2, 2}, {0, 2, 2, 2}}},
}

// FindTemplate returns a built-in template by name.
func FindTemplate(name string) (Template, bool) {
	for _, template := range Templates {
		if template.Name == name {
			return template, true
		}
	}
	return Template{}, false
}

// Places returns the big cells first, then every remaining single cell
// row by row, as the frozen source orders them.
func (t Template) Places() [][4]int {
	covered := map[[2]int]bool{}
	for _, big := range t.Big {
		for row := big[0]; row < big[0]+big[2]; row++ {
			for col := big[1]; col < big[1]+big[3]; col++ {
				covered[[2]int{row, col}] = true
			}
		}
	}
	places := append([][4]int(nil), t.Big...)
	for row := 0; row < t.Rows; row++ {
		for col := 0; col < t.Cols; col++ {
			if !covered[[2]int{row, col}] {
				places = append(places, [4]int{row, col, 1, 1})
			}
		}
	}
	return places
}

// TemplateLayout lays out n cells in the template's places.
func TemplateLayout(n int, area Rect, gap float64, template Template) []Rect {
	if n <= 0 || area.W <= 0 || area.H <= 0 {
		return nil
	}
	places := template.Places()
	if n > len(places) {
		return nil
	}
	gap = max(gap, 0)
	cellW := (area.W - gap*float64(template.Cols-1)) / float64(template.Cols)
	cellH := (area.H - gap*float64(template.Rows-1)) / float64(template.Rows)
	if cellW <= 0 || cellH <= 0 {
		return nil
	}
	out := make([]Rect, n)
	for i := 0; i < n; i++ {
		place := places[i]
		out[i] = Rect{
			X: area.X + float64(place[1])*(cellW+gap),
			Y: area.Y + float64(place[0])*(cellH+gap),
			W: float64(place[3])*cellW + float64(place[3]-1)*gap,
			H: float64(place[2])*cellH + float64(place[2]-1)*gap,
		}
	}
	return out
}
