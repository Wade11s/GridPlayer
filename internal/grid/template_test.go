package grid

import "testing"

func TestTemplatePlaces(t *testing.T) {
	template, ok := FindTemplate("1 + 2")
	if !ok {
		t.Fatal("missing template")
	}
	places := template.Places()
	if len(places) != 3 {
		t.Fatalf("places %d", len(places))
	}
	if places[0] != [4]int{0, 0, 2, 2} {
		t.Fatalf("big %v", places[0])
	}
	if places[1] != [4]int{0, 2, 1, 1} || places[2] != [4]int{1, 2, 1, 1} {
		t.Fatalf("singles %v", places[1:])
	}
}

func TestTemplateLayout(t *testing.T) {
	template, _ := FindTemplate("1 + 2")
	rects := TemplateLayout(3, Rect{W: 300, H: 200}, 0, template)
	if len(rects) != 3 {
		t.Fatalf("rects %d", len(rects))
	}
	if rects[0] != (Rect{X: 0, Y: 0, W: 200, H: 200}) {
		t.Fatalf("big %+v", rects[0])
	}
	if rects[1] != (Rect{X: 200, Y: 0, W: 100, H: 100}) {
		t.Fatalf("single 1 %+v", rects[1])
	}
	if rects[2] != (Rect{X: 200, Y: 100, W: 100, H: 100}) {
		t.Fatalf("single 2 %+v", rects[2])
	}
	if TemplateLayout(4, Rect{W: 300, H: 200}, 0, template) != nil {
		t.Fatal("overflow accepted")
	}
}
