package unit

import (
	"testing"

	etmath "github.com/jerbe/et-go/engine/math"
)

func assertFloat3Close(t *testing.T, got, want etmath.Vector3) {
	t.Helper()
	if got.Distance(want) > 1e-5 {
		t.Fatalf("Float3 = %v, want %v", got, want)
	}
}

func assertQuaternionClose(t *testing.T, got, want etmath.Quaternion) {
	t.Helper()
	if etmath.QuaternionDistance(got, want) > 1e-5 {
		t.Fatalf("Quaternion = %v, want %v", got, want)
	}
}

func TestFloat3Operations(t *testing.T) {
	assertFloat3Close(t, etmath.Vector3{X: 1}.Add(etmath.Vector3{Y: 1}), etmath.Vector3{X: 1, Y: 1})
	assertFloat3Close(t, etmath.Vector3{X: 1, Y: 2, Z: 3}.Sub(etmath.Vector3{X: 1, Y: 1}), etmath.Vector3{Y: 1, Z: 3})
	assertFloat3Close(t, etmath.Vector3{X: 2, Y: 3}.Mul(2), etmath.Vector3{X: 4, Y: 6})
	assertFloat3Close(t, etmath.Lerp(etmath.Vector3Zero, etmath.Vector3{X: 10}, 0.5), etmath.Vector3{X: 5})
	assertFloat3Close(t, etmath.Vector3Zero.Normalize(), etmath.Vector3Zero)
	if got := (etmath.Vector3{X: 3, Y: 4}).Distance(etmath.Vector3Zero); got != 5 {
		t.Fatalf("Distance = %v, want 5", got)
	}
}

func TestQuaternionOperations(t *testing.T) {
	assertFloat3Close(t, etmath.QuaternionForward(etmath.QuaternionIdentity), etmath.Vector3Forward)

	a := etmath.QuaternionIdentity
	b := etmath.LookRotation(etmath.Vector3{X: 1, Y: 0, Z: 0})
	assertQuaternionClose(t, etmath.Slerp(a, b, 0), a)
	assertQuaternionClose(t, etmath.Slerp(a, b, 1), b)
	assertQuaternionClose(t, etmath.LookRotation(etmath.Vector3Forward), etmath.QuaternionIdentity)
	assertFloat3Close(t, etmath.QuaternionForward(b), etmath.Vector3{X: 1, Y: 0, Z: 0})
}
