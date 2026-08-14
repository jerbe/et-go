package math

import "testing"

func assertFloat3Approx(t *testing.T, got, want Vector3) {
	t.Helper()
	if got.Distance(want) > 1e-5 {
		t.Fatalf("Float3 = %v, want %v", got, want)
	}
}

func assertQuaternionApprox(t *testing.T, got, want Quaternion) {
	t.Helper()
	if QuaternionDistance(got, want) > 1e-5 {
		t.Fatalf("Quaternion = %v, want %v", got, want)
	}
}

func TestFloat3Helpers(t *testing.T) {
	assertFloat3Approx(t, Vector3{X: 1}.Add(Vector3{Y: 1}), Vector3{X: 1, Y: 1})
	assertFloat3Approx(t, Vector3{X: 3, Y: 2}.Sub(Vector3{X: 1, Y: 1}), Vector3{X: 2, Y: 1})
	assertFloat3Approx(t, Vector3{X: 2, Y: 3, Z: 4}.Mul(2), Vector3{X: 4, Y: 6, Z: 8})
	assertFloat3Approx(t, Lerp(Vector3Zero, Vector3{X: 10}, 0.5), Vector3{X: 5})
	assertFloat3Approx(t, Vector3Zero.Normalize(), Vector3Zero)
	if got := (Vector3{X: 3, Y: 4}).Distance(Vector3Zero); got != 5 {
		t.Fatalf("Distance = %v, want 5", got)
	}
	if Zero() != Vector3Zero {
		t.Fatal("Zero should return Float3Zero")
	}
}

func TestQuaternionHelpers(t *testing.T) {
	assertFloat3Approx(t, QuaternionForward(QuaternionIdentity), Vector3Forward)

	right := LookRotation(Vector3{X: 1, Y: 0, Z: 0})
	assertFloat3Approx(t, QuaternionForward(right), Vector3{X: 1, Y: 0, Z: 0})

	assertQuaternionApprox(t, Slerp(QuaternionIdentity, right, 0), QuaternionIdentity)
	assertQuaternionApprox(t, Slerp(QuaternionIdentity, right, 1), right)
	assertQuaternionApprox(t, LookRotation(Vector3Forward), QuaternionIdentity)
	if QuaternionIdentity.String() == "" || Vector3Zero.String() == "" {
		t.Fatal("String helpers should return readable text")
	}
}
