package move

import (
	"errors"
	"testing"

	etmath "github.com/jerbe/et-go/engine/math"
)

type fakeFinder struct {
	start    etmath.Vector3
	target   etmath.Vector3
	extents  etmath.Vector3
	maxPolys int
	result   []etmath.Vector3
}

func (f *fakeFinder) FindPath(start, target, extents etmath.Vector3, maxPolys int) ([]etmath.Vector3, error) {
	f.start = start
	f.target = target
	f.extents = extents
	f.maxPolys = maxPolys
	return append([]etmath.Vector3(nil), f.result...), nil
}

func TestPathfindingComponentFind(t *testing.T) {
	finder := &fakeFinder{
		result: []etmath.Vector3{
			{X: -1, Y: 0, Z: 2},
			{X: -3, Y: 0, Z: 4},
		},
	}
	component := &PathfindingComponent{}
	component.SetFinder(finder)

	path, err := component.Find(etmath.Vector3{X: 1, Y: 0, Z: 2}, etmath.Vector3{X: 3, Y: 0, Z: 4})
	if err != nil {
		t.Fatalf("Find error = %v", err)
	}
	if finder.start.X != -1 || finder.target.X != -3 {
		t.Fatalf("finder input conversion mismatch: start=%v target=%v", finder.start, finder.target)
	}
	if finder.extents != (etmath.Vector3{X: NavExtentX, Y: NavExtentY, Z: NavExtentZ}) {
		t.Fatalf("extents = %v, want defaults", finder.extents)
	}
	if finder.maxPolys != NavMaxPolys {
		t.Fatalf("maxPolys = %d, want %d", finder.maxPolys, NavMaxPolys)
	}
	if len(path) != 2 || path[0].X != 1 || path[1].X != 3 {
		t.Fatalf("converted path = %v, want x flipped back", path)
	}
}

func TestPathfindingComponentRequiresFinder(t *testing.T) {
	component := &PathfindingComponent{}
	if _, err := component.Find(etmath.Vector3{}, etmath.Vector3{X: 1}); err != ErrFinderMissing {
		t.Fatalf("Find error = %v, want %v", err, ErrFinderMissing)
	}
}

func TestNilPathfindingComponentReturnsError(t *testing.T) {
	var component *PathfindingComponent
	if _, err := component.Find(etmath.Vector3{}, etmath.Vector3{X: 1}); !errors.Is(err, ErrFinderMissing) {
		t.Fatalf("nil component error = %v, want %v", err, ErrFinderMissing)
	}
}

func TestNewFinderRequiresRegisteredFactory(t *testing.T) {
	RegisterFinderFactory(nil)
	if _, err := NewFinder("Map1", "Map1.nav"); !errors.Is(err, ErrFinderFactoryMissing) {
		t.Fatalf("NewFinder error = %v, want %v", err, ErrFinderFactoryMissing)
	}
}

func TestNewFinderRejectsNilFactoryResult(t *testing.T) {
	t.Cleanup(func() {
		RegisterFinderFactory(nil)
	})
	RegisterFinderFactory(func(string, string) (Finder, error) {
		return nil, nil
	})
	if _, err := NewFinder("Map1", "Map1.nav"); !errors.Is(err, ErrFinderFactoryFailed) {
		t.Fatalf("NewFinder error = %v, want %v", err, ErrFinderFactoryFailed)
	}
}
